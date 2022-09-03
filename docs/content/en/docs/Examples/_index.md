
---
title: "Examples"
linkTitle: "Examples"
weight: 3
date: 2017-01-05
description: >
  See your project in action!
---

## Grant Port Forward Accross Namespaces

This example walks you through defining a `ClusterRole` and an `EscalationPolicy` that allows your user to temporarily
get the port-forward permission on two different namespaces.

First, let's define a [ClusterRole](https://kubernetes.io/docs/reference/access-authn-authz/rbac/#clusterrole-example) that grants `exec` on pods.

```yaml
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  name: port-forwarder
rules:
- apiGroups: [""]
  resources: ["pods/portforward"]
  verbs: ["create"]
```

Defining a cluster role by itself doesn't do much. Let's define an [EscalationPolicy](./) to allow our users to use this new-role.

```yaml
apiVersion: k8s.kudo.dev/v1alpha1
kind: EscalationPolicy
metadata:
  name: gain-port-forward
spec:
  subjects:
    - kind: Group
      name: system:authenticated # All the authenticated users.
  challenges:
    - kind: PeerReview
      reviewers:
        - kind: Group
          name: admin@my-company.io
  target:
    duration: 60m
    grants:
    - kind: KubernetesRoleBinding
      defaultNamespace: application-a
      allowedNamespaces:
        - application-a
        - application-b
      roleRef:
        kind: ClusterRole
        name: port-forwarder
        apiGroup: rbac.authorization.k8s.io
```

Let's review the configuration This [EscalationPolicy](./) translates to the following:

- The `subjects` section tells that all the `authenticated` users are allowed to escalate using this policy
- The `challenges` sections defines that for this escalatio to work, escalation must be approved by one member of the group `admin@my-company.io`
- The `target` sections defines what the escalation actually grants:
  - Kudo will create a `RoleBinding` for the requestor to the role `port-forwarder` by default in the `application-a` namespace, but is allowed to be used in the `application-a` and `application-b` namespace.
  - The escalation will last 60 minutes.


From there, you use your new policy using kudo kubectl plugin:

```bash
kubectl kudo escalate gain-port-forward --namespace application-b "need to debug application B, ticket #3939"
```
