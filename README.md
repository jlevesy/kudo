# kudo: sudo for Kubernetes

## What is Kudo

Kudo is a Kubernetes controller that allows individual users to temporarily escalate their permissions while still maintaining security and crystal clear auditability.

It comes in complement of existing access control systems (Kubernetes RBAC, GCP IAM), and relies on them to temporarily grant or reclaim permissions. In the context of Kubernetes, Kudo temporarily creates a `RoleBinding` or a `ClusterRoleBinding` between an existing role and the escalation requestor.

To gain a better understanding of what Kudo is, you can refer to [the Kudo website](https://jlevesy.github.io/kudo)

## Project Status

This is a toy project at the moment, please do not try to use it in that state as It can be dangerous from a security standpoint and I don't provide support in any case.

Though, if you like the idea, please let me know!

## Contributing To Kudo

### Roadmap

The [kudo project](https://github.com/users/jlevesy/projects/1) pretty much carries all what I have in mind for kudo. Feel free to pick a task in the `TODO` column.

### Tooling

Here's a list of tools you need to have installed to run your development environment.

- [go1.19](https://go.dev/learn/)
- [k3d](https://github.com/k3d-io/k3d)
- [ko](https://github.com/google/ko)
- [helm](https://helm.sh/)
- [kubectl](https://kubernetes.io/docs/tasks/tools/#kubectl)
- OpenSSL
- (For the documentation) [hugo](https://gohugo.io/)

### Configuration

You need to have both `kudo-registry.localhost` and `kudo-e2e-registry.localhost` configured to resolve to 127.0.0.1 in your development environment.

### Runing the Development Environment

The folowing command line spins up a k3d cluster, provision necessary resources and install your current tree version of kudo in the cluster.

```bash
make run_dev
```

To simulate an escalation. This will switch your current kubectl context to the kudo test user, create the permission, then switch back to the admin context.

```bash
make run_escalation_dev
```

To display the controller logs

```bash
make logs_dev
```

### Runing the Test Suite

- `make unit_tests` runs the unit test suite, ie all the tests defined in package.
- `make e2e_tests` runs the end to end test suite, which simulate real kudo use cases. The test suite lives under the `./e2e` directory.

### Serving The Documentation

- `make serve_docs` starts a local webserver. You can then go to `http://localhost:1313/kudo` to check your local doc.
