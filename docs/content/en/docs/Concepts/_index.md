---
title: "Concepts"
linkTitle: "Concepts"
weight: 4
description: >
  Gain a better understanding of Kudo building blocks!
---

## Core Concepts

### Escalation Policy

An escalation policy define a possible path to escalation. It is composed by the following sections:

- `subjects`: list of principals allowed to use the policy. A principal is expressed as a `Kind` (being potentially `Group` or `User`) and a name which could be either an user identifier or a Kubernetes group name. This is the same model than the one Kubernetes RBAC uses for `ClusterRoleBindings` and `RoleBindings`.
- `challenges`: Expresses a list of verifications that have to be performed at escalation time. For example, this where you specify that an escalations needs to be peer reviewed by a member of another group.
- `target`: Defines what the escalation actually grants. It is composed by common settings like how much time this escalation is actually valid and also a one or more  esclation grants, which represent an action to be done to actually grant permissions. For example, the escalation grant `KubernetesRoleBinding` tells Kudo to create a role binding in the requested namespace.

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
    - kind: PeerReview
      reviewers:
        - kind: Group
          name: squad-b@voiapp.io
  target: # (required) what the escalation grants
    duration: 60m
    grants:
    - kind: KubernetesRoleBinding
      defaultNamespace: some-app
      allowedNamespaces:
        - some-app
        - some-other-app
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
  - `namespace`: (optional) a namespace requested by the user.

- `status`: current status of the escalation:
  - `state`:
    - `PENDING`: the escalation is awaiting challenge completion
    - `DENIED`: user isn't allowed to escalate, one of the challenges has failed or Kudo has defensively decided to deny the escalation.
    - `ACCEPTED`: the escalation is accepted and the user has now access to extended privileges
    - `EXPIRED`: the escalation has expired
  - `stateDetails`: some aditional information regarding the state
  - `PolicyUID` and `PolicyResourceVersion`: which policy resource instance is this escalation based on.
  - `ExpiresAt` when the escalation expires
- `GrantRefs`: List of references to all the resource being granted by Kudo with their status.

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
  policyUID: aaa-bb-cc
  policyVersion: 484
  grantRefs:
    - kind: KubernetesRoleBinding
      name: binding-343df
      namespace: some-app
      UID: aaaa-bbb-ccc
      ResourceVersion: 493
      status: "CREATED"
```
