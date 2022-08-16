package e2e

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"testing"

	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/klog/v2"

	kudoclientset "github.com/jlevesy/kudo/pkg/generated/clientset/versioned"
)

const (
	registryDomain = "kudo-e2e-registry.localhost"
	imageRef       = registryDomain + ":5000/kudo"
	k3dClusterName = "kudo-e2e-test"
)

var (
	k8sClientset  kubernetes.Interface
	kudoClientSet kudoclientset.Interface

	debug, _ = strconv.ParseBool(os.Getenv("DEBUG"))
)

func TestMain(m *testing.M) {
	os.Exit(run(m))
}

func run(m *testing.M) int {
	ctx := context.Background()

	klog.Info("Checking system dependencies")
	if err := checkBinaries("k3d", "ko", "helm"); err != nil {
		klog.ErrorS(err, "One or more necessary binaries are not found")
		return 1
	}

	if err := checkHostsFile(registryDomain); err != nil {
		klog.ErrorS(err, "/etc/hosts configuration required")
		return 1
	}

	klog.Info("/etc/hosts file properly configured")

	klog.Info("Booting a k3d cluster")
	if err := hasK3dCluster(ctx, k3dClusterName); err == nil {
		klog.Info("Found an existing cluster, cleaning it up")

		if err := deleteK3dCluster(ctx, k3dClusterName); err != nil {
			klog.ErrorS(err, "unable to delete previous k3d cluster")
			return 1
		}
	}

	if err := runK3dCluster(ctx, k3dClusterName); err != nil {
		klog.ErrorS(err, "Unable to create a k3d cluster")
		return 1
	}

	defer func() {
		klog.Info("Cleaning k3d cluster")
		if err := deleteK3dCluster(ctx, k3dClusterName); err != nil {
			klog.ErrorS(err, "Unable to delete a k3d cluster")
		}
	}()

	kubeConfigFile, err := getKubeConfig(ctx, k3dClusterName)
	if err != nil {
		klog.ErrorS(err, "Unable to retrieve kube config")
		return 1
	}

	klog.InfoS("kubeconfig can be found here", "path", kubeConfigFile.Name())

	defer os.Remove(kubeConfigFile.Name())

	klog.Info("Building controller image")
	if err := buildAndPushImage(ctx); err != nil {
		klog.ErrorS(err, "Unable to build image")
		return 1
	}

	k8sClientset, kudoClientSet, err = buildK8sClientSets(ctx, kubeConfigFile.Name())
	if err != nil {
		klog.ErrorS(err, "Unable to build k8s clients")
		os.Exit(1)
	}

	klog.Info("Deploying kudo")
	if err := installKudo(ctx, kubeConfigFile.Name()); err != nil {
		klog.ErrorS(err, "Unable to install kudo")
		return 1
	}

	klog.Info("Runing tests")
	return m.Run()
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
				"--namespace=kudo-e2e",
				"--set=image.devRef=" + imageRef,
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

func buildK8sClientSets(ctx context.Context, kubeFilePath string) (kubernetes.Interface, kudoclientset.Interface, error) {
	kubeCfg, err := clientcmd.BuildConfigFromFlags(
		"",
		kubeFilePath,
	)
	if err != nil {
		return nil, nil, err
	}

	kubeClient, err := kubernetes.NewForConfig(kubeCfg)
	if err != nil {
		return nil, nil, err
	}

	kudoClient, err := kudoclientset.NewForConfig(kubeCfg)
	if err != nil {
		return nil, nil, err
	}

	return kubeClient, kudoClient, nil
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
