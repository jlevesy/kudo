package e2e

import (
	"bufio"
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/asn1"
	"encoding/pem"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"

	certificatesv1 "k8s.io/api/certificates/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	restclient "k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/klog/v2"

	kudov1alpha1 "github.com/jlevesy/kudo/pkg/apis/k8s.kudo.dev/v1alpha1"
	kudoclientset "github.com/jlevesy/kudo/pkg/generated/clientset/versioned"
	"github.com/jlevesy/kudo/pkg/generics"
)

const (
	registryDomain       = "kudo-e2e-registry.localhost"
	imageRef             = registryDomain + ":5000/kudo"
	k3dClusterName       = "kudo-e2e-test"
	kudoInstallNamespace = "kudo-e2e"
)

var (
	admin clientSet
	userA clientSet
)

var (
	debug, _ = strconv.ParseBool(os.Getenv("DEBUG"))
)

func dumpControllerLogs(ctx context.Context) error {
	pods, err := admin.k8s.CoreV1().Pods(kudoInstallNamespace).List(
		ctx,
		metav1.ListOptions{
			LabelSelector: "app.kubernetes.io/name=kudo",
		},
	)
	if err != nil {
		return err
	}

	for _, pod := range pods.Items {
		klog.InfoS("===============> Logs from", "pod", pod.Name)

		logs, err := admin.k8s.CoreV1().Pods(kudoInstallNamespace).GetLogs(
			pod.Name,
			&corev1.PodLogOptions{},
		).Stream(ctx)
		if err != nil {
			return err
		}

		defer logs.Close()

		_, _ = io.Copy(os.Stdout, logs)

		klog.InfoS("===============> End Logs from", "pod", pod.Name)
	}

	return nil
}

func buildAdminClientSet(kubeConfigPath string) (clientSet, error) {
	kubeRestCfg, err := clientcmd.BuildConfigFromFlags(
		"",
		kubeConfigPath,
	)
	if err != nil {
		return clientSet{}, err
	}

	return buildClientSet(kubeRestCfg, "system:admin")
}

func provisionUsersPermissions(ctx context.Context) error {
	var (
		canEscalateClusterRole = rbacv1.ClusterRole{
			ObjectMeta: metav1.ObjectMeta{
				Name: "can-escalate",
			},
			Rules: []rbacv1.PolicyRule{
				{
					Verbs: []string{"create"},
					APIGroups: []string{
						kudov1alpha1.SchemeGroupVersion.Group,
					},
					Resources: []string{"escalations"},
				},
			},
		}
		canEscalateRoleBinding = rbacv1.ClusterRoleBinding{
			ObjectMeta: metav1.ObjectMeta{
				Name: "can-escalate",
			},
			Subjects: []rbacv1.Subject{
				{
					Kind: rbacv1.GroupKind,
					Name: "system:authenticated", // all authenticated users can escalate.
				},
			},
			RoleRef: rbacv1.RoleRef{
				APIGroup: rbacv1.SchemeGroupVersion.Group,
				Kind:     "ClusterRole",
				Name:     "can-escalate",
			},
		}
	)

	_, err := admin.k8s.RbacV1().ClusterRoles().Create(ctx, &canEscalateClusterRole, metav1.CreateOptions{})
	if err != nil {
		return err
	}

	_, err = admin.k8s.RbacV1().ClusterRoleBindings().Create(ctx, &canEscalateRoleBinding, metav1.CreateOptions{})
	return err

}

// This is vastly inspired from https://github.com/abohmeed/k8susercreator/blob/master/cmd/k8suser/main.go
func generateK8sUser(ctx context.Context, name, group string) (clientSet, error) {
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return clientSet{}, err
	}

	subject := pkix.Name{
		CommonName:   name,
		Organization: []string{group},
	}

	asn1Subject, err := asn1.Marshal(subject.ToRDNSequence())
	if err != nil {
		return clientSet{}, err
	}

	csrReq := x509.CertificateRequest{
		RawSubject:         asn1Subject,
		SignatureAlgorithm: x509.SHA256WithRSA,
	}

	csrBytes, err := x509.CreateCertificateRequest(rand.Reader, &csrReq, privateKey)
	if err != nil {
		return clientSet{}, err
	}

	var (
		csrName   = name + "-csr"
		csrClient = admin.k8s.CertificatesV1().CertificateSigningRequests()
	)

	// Submit CSR and approve it
	k8sCsr, err := csrClient.Create(
		ctx,
		&certificatesv1.CertificateSigningRequest{
			ObjectMeta: metav1.ObjectMeta{
				Name: csrName,
			},
			Spec: certificatesv1.CertificateSigningRequestSpec{
				SignerName: "kubernetes.io/kube-apiserver-client",
				Usages:     []certificatesv1.KeyUsage{certificatesv1.UsageClientAuth},
				Request: pem.EncodeToMemory(
					&pem.Block{Type: "CERTIFICATE REQUEST", Bytes: csrBytes},
				),
				ExpirationSeconds: generics.Ptr(int32(86400)),
			},
		},
		metav1.CreateOptions{},
	)

	if err != nil {
		return clientSet{}, err
	}

	k8sCsr.Status.Conditions = append(k8sCsr.Status.Conditions, certificatesv1.CertificateSigningRequestCondition{
		Type:           certificatesv1.CertificateApproved,
		Status:         corev1.ConditionTrue,
		Reason:         "User activation",
		Message:        "SEAL OF APPROVAL",
		LastUpdateTime: metav1.Now(),
	})

	k8sCsr, err = csrClient.UpdateApproval(ctx, k8sCsr.GetName(), k8sCsr, metav1.UpdateOptions{})
	if err != nil {
		return clientSet{}, err
	}

	// Get back the signed certificate. Retry until we get it.
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	for len(k8sCsr.Status.Certificate) == 0 {
		time.Sleep(time.Second)
		klog.Info("Retrieving user certificate")
		k8sCsr, err = csrClient.Get(ctx, k8sCsr.GetName(), metav1.GetOptions{})
		if err != nil {
			return clientSet{}, err
		}
	}

	// Shallow copy the admin kubeconfig and replace the certs by the one granted to our new user.
	restConfig := admin.cfg

	restConfig.TLSClientConfig.CertData = k8sCsr.Status.Certificate
	restConfig.TLSClientConfig.KeyData = pem.EncodeToMemory(
		&pem.Block{
			Type:  "RSA PRIVATE KEY",
			Bytes: x509.MarshalPKCS1PrivateKey(privateKey),
		},
	)

	return buildClientSet(&restConfig, name)
}

func installKudo(ctx context.Context, kubeConfigPath string) error {
	return execCmd(
		ctx,
		execCall{
			name: "helm",
			args: []string{
				"upgrade",
				"--install",
				"--kubeconfig=" + kubeConfigPath,
				"--create-namespace",
				"--namespace=" + kudoInstallNamespace,
				"--set=image.devRef=" + imageRef,
				"--set=controller.resyncInterval=5s",
				"--set=controller.retryInterval=1s",
				"--set=resources.limits.cpu=1",
				"--set=resources.requests.cpu=1",
				"--wait",
				"--timeout=1m",
				"kudo-e2e",
				"../helm",
			},
		},
	)
}

func buildAndPushImage(ctx context.Context) error {
	return execCmd(
		ctx,
		execCall{
			name: "ko",
			env: map[string]string{
				"KO_DOCKER_REPO": registryDomain + ":5001/kudo",
			},
			args: []string{
				"build",
				"--bare",
				"--insecure-registry",
				"../cmd/controller",
			},
		},
	)
}

func getKubeConfig(ctx context.Context, clusterName string) (*os.File, error) {
	kubeConfigFile, err := os.CreateTemp("", "kubeconfig")
	if err != nil {
		return nil, err
	}

	return kubeConfigFile, execCmd(
		ctx,
		execCall{
			name: "k3d",
			args: []string{
				"kubeconfig",
				"write",
				clusterName,
				"--output=" + kubeConfigFile.Name(),
			},
		},
	)
}

type clientSet struct {
	k8s      kubernetes.Interface
	kudo     kudoclientset.Interface
	cfg      restclient.Config
	userName string
}

func buildClientSet(cfg *restclient.Config, userName string) (clientSet, error) {
	kubeClient, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		return clientSet{}, err
	}

	kudoClient, err := kudoclientset.NewForConfig(cfg)
	if err != nil {
		return clientSet{}, err
	}

	return clientSet{
		k8s:      kubeClient,
		kudo:     kudoClient,
		cfg:      *cfg,
		userName: userName,
	}, nil
}

func runK3dCluster(ctx context.Context, clusterName string) error {
	return execCmd(
		ctx,
		execCall{
			name: "k3d",
			args: []string{
				"cluster",
				"create",
				clusterName,
				"--image=rancher/k3s:v1.24.3-k3s1",
				fmt.Sprintf("--registry-create=%s:0.0.0.0:5001", registryDomain),
				"--no-lb",
				"--kubeconfig-switch-context=false",
				"--kubeconfig-update-default=false",
				"--k3s-arg",
				"--disable=traefik@server:0",
			},
		},
	)
}

func hasK3dCluster(ctx context.Context, clusterName string) error {
	return execCmd(
		ctx,
		execCall{
			name: "k3d",
			args: []string{
				"cluster",
				"get",
				clusterName,
			},
		},
	)
}

func deleteK3dCluster(ctx context.Context, clusterName string) error {
	return execCmd(
		ctx,
		execCall{
			name: "k3d",
			args: []string{
				"cluster",
				"delete",
				clusterName,
			},
		},
	)
}

type execCall struct {
	name string
	args []string
	env  map[string]string
}

func execCmd(ctx context.Context, call execCall) error {
	var (
		cmd = exec.CommandContext(ctx, call.name, call.args...)
	)

	cmd.Env = os.Environ()

	for varName, value := range call.env {
		cmd.Env = append(cmd.Env, varName+"="+value)
	}

	if debug {
		klog.Info("Running command: ", cmd.String())

		stdout, err := cmd.StdoutPipe()
		if err != nil {
			return err
		}
		defer stdout.Close()

		stderr, err := cmd.StderrPipe()
		if err != nil {
			return err
		}
		defer stderr.Close()

		copyDone := make(chan struct{})
		// Always wait for the stdout copy  to be done at that point.
		defer func() { <-copyDone }()

		go func() {
			_, _ = io.Copy(os.Stdout, io.MultiReader(stdout, stderr))
			close(copyDone)
		}()
	}

	if err := cmd.Run(); err != nil {
		return err
	}

	return nil
}

func checkBinaries(binaries ...string) error {
	for _, binary := range binaries {
		path, err := exec.LookPath(binary)
		if err != nil {
			return err
		}

		klog.InfoS("Using binary", "name", binary, "path", path)
	}

	return nil
}

func checkHostsFile(registryDomain string) error {
	file, err := os.Open("/etc/hosts")
	if err != nil {
		return err
	}

	defer file.Close()

	scanner := bufio.NewScanner(file)

	for scanner.Scan() {
		if strings.Contains(scanner.Text(), registryDomain) && strings.Contains(scanner.Text(), "127.0.0.1") {
			return nil
		}
	}

	if err := scanner.Err(); err != nil {
		return err
	}

	return fmt.Errorf("domain %s can't be found in your /etc/hosts file, please configure it to point to 127.0.0.1", registryDomain)
}
