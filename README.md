# kudo: sudo for Kubernetes

## Project Status

This is a toy project at the moment, please do not try to use it in that state as It can be dangerous from a security standpoint and I don't provide support in any case.

Though, if you like the idea, please let me know!

## Problem Statement

In large organisations, it is common practice to make sure that the right persons have access to only what they need to do their work. This is usualy implemented by systems using some kind of role based access control (in other words being able to grant to a user certain roles granting permissions resources. Kubernetes provides RBAC for this, other cloud providers (GoogleCloud, AWS...) have comparable systems, and so on so forth.

Now, designing a permission setup that protects your organisation while maintaining a good level of usablility is a difficult exercise. As Human organisations tends to always change, setting strict boundaries often ends up slowing down people and sometimes becomes a real problem.

This is the well known tradeoff between security and usability, so how do we solve that? Allow your users to escalate their permission temporarily, under supervision!

Ecalation isn't something that should be taken lightly, otherwise what's the point of doing access control!

For example we would like to make sure that the following aspects are enforced:

- The user is allowed to escalate
- The escalation is potentially validated by some third party
- The escalation is traced securely for observability.

And this is where Kudo comes into play! This Kubernetes controller aims to;

- Make cluster administrators able to define `EscalationPolicies`. In other words: define who has the right to escalate, what needs to be checked before accepting the escalation and what the escalation actually grants. Policies allows to fine tune permissions escalations to provide the right flexibility while maintaining security.
- Make users able to escalate their permissions easily, using nothing more than `kubectl kudo escalate`.

## How it works?

- Cluster admins define one or many `EscalationPolicy`
- Cluster users create escalations refering a policy. `kubectl create escalation --policy=some-policy-name` (or `kubectl kudo escalate some-policy-name`)
- This creates an Escalation object that is picked up by the kudo-controller. From there the controller:
  - Makes sure that the escalating user is allowed to use the escalation policy.
  - Run the policies `challenges` to verify at runtime that the user is allowed to perform the escalation.
  - Create the necessary resource(s) to grant the escalation.
  - Whatever the outcome of the escalation attempt, it gets logged into k8s and optionnaly into a secure storage.

## CRDs

### EscalationPolicy

An escalation policy define a possible path to escalation. It is composed by the following sections:

- `subjects`: list of principals allowed to use the policy. Type is `[]rbac.authorization.k8s.io.Subject`
- `challenges`: list of runtime challenges to run when an user attempts to escalate.
- `target`: defines what's granted by the escalation and for how much time.


```yaml
# Grants any members of squad-a the authorization to gain the RBAC role `some-escalated-role`
# on the namespace `some-app` for 60 minutes if and only if a member of squad-b approves the escalation
---
apiVersion: k8s.kudo.dev/v1alpha1
kind: EscalationPolicy
metadata:
  name: rbac-escalation-example
spec:
  subjects: # (required) who has the right to trigger this escalation.
    - kind: Group
      name: squad-a@group.com
  challenges: # (optional) list of challenges being applied when esclating.
    - kind: TwoFactor
      reviewers:
        - kind: Group
          name: squad-b@voiapp.io
  target: # (required) what the escalation grants
    duration: 60m
    grants:
    - kind: KubernetesRoleBinding
      namespace: some-app
      roleRef:
        kind: ClusterRole
        name: some-escalated-role
        apiGroup: rbac.authorization.k8s.io
```

### Escalation

An escalation represents the actual demand of permission escalation by an user.

It is composed by the following attributes:

- `spec`: spec of the escalation
  - `policyName`: name of the policy being used to escalate
  - `requestor`: identifier of the user asking for permission escalation
  - `reason`: a reason to explain why the user is asking to escalate their permissions

- `status`: current status of the escalation:
  - `state`:
    - `PENDING`: the escalation is awaiting challenge completion
    - `DENIED`: user isn't allowed to escalate, or one of the challenges has failed.
    - `ACCEPTED`: the escalation is accepted and the user has now access to extended privileges
    - `EXPIRED`: the escalation has expired
  - `stateDetails`: some aditional information regarding the state
- `targetRef`: Reference of the resource being created to grant permissions.

```yaml
---
apiVersion: k8s.kudo.dev/v1alpha1
kind: Escalation
metadata:
  name: escalation-abbdfff3
spec:
  policyName: rbac-escalation-exaple
  requestor: user-1@kubecluster.com
  reason: "Needs access to squad-b namespace to debug my service"
status:
  state: "ACCEPTED"
  stateDetails: "Escalation accepted, all resources are created"
  targetRefs:
    - kind: KubernetesRoleBinding
      name: binding-343df
      namespace: some-app
      UID: aaaa-bbb-ccc
      ResourceVersion: 493
      status: "CREATED"

```

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

### Configuration

You need to have both `kudo-registry.localhost` and `kudo-e2e-registy.localhost` configured to resolve to 127.0.0.1 in your development environment.

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

- `make unit_tests` runs the unit test suite, ie all the tests defined in package. Those thests aims to be exhaustive and fast.
- `make e2e_tests` runs the end to end test suite, which simulate real kudo use cases. They live under the `./e2e` directory.
