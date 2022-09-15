package e2e

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"testing"

	"golang.org/x/sync/errgroup"
	"k8s.io/klog/v2"
)

var (
	nok3dCleanup, _ = strconv.ParseBool(os.Getenv("NO_K3D_CLEANUP"))
	k3sVersion      = os.Getenv("K3S_VERSION")
)

func TestMain(m *testing.M) {
	os.Exit(run(m))
}

func run(m *testing.M) int {
	ctx := context.Background()
	defer klog.Flush()

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

	klog.Info("Booting a k3d cluster, with k3s version: ", k3sVersion)
	if err := hasK3dCluster(ctx, k3dClusterName); err == nil {
		klog.Info("Found an existing cluster, cleaning it up")

		if err := deleteK3dCluster(ctx, k3dClusterName); err != nil {
			klog.ErrorS(err, "unable to delete previous k3d cluster")
			return 1
		}
	}

	if err := runK3dCluster(ctx, k3dClusterName, k3sVersion); err != nil {
		klog.ErrorS(err, "Unable to create a k3d cluster")
		return 1
	}

	defer func() {
		if nok3dCleanup {
			return
		}

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

	admin, err = buildAdminClientSet(kubeConfigFile.Name())
	if err != nil {
		klog.ErrorS(err, "Unable to build k8s clients")
		return 1
	}

	klog.Info("Building controller image")
	if err := buildAndPushImage(ctx); err != nil {
		klog.ErrorS(err, "unable to build image")
		return 1
	}

	var group errgroup.Group

	group.Go(func() error {
		klog.Info("Provisioning user permissions")
		if err := provisionUsersPermissions(ctx); err != nil {
			return fmt.Errorf("unable to provision permissions: %w", err)
		}

		return nil
	})

	group.Go(func() error {
		klog.Info("Generating user credentials")

		var err error

		userA, err = generateK8sUser(ctx, "user-A", "group-A")
		if err != nil {
			return fmt.Errorf("unable to generate user: %w", err)
		}

		return err
	})

	defer func() {
		if err := dumpControllerLogs(ctx); err != nil {
			klog.ErrorS(err, "Unable to dump controler logs")
		}
	}()

	group.Go(func() error {
		klog.Info("Installing kudo")
		if err := installKudo(ctx, kubeConfigFile.Name()); err != nil {
			return fmt.Errorf("unable to install kudo: %w", err)
		}

		return nil
	})

	if err := group.Wait(); err != nil {
		klog.ErrorS(err, "one of the setup step has failed, cancelling run")
		return 1
	}

	klog.Info("Runing tests")
	return m.Run()
}
