# kudo: sudo for Kubernetes

## Problem Statement

In large organisations, it is common practice to make sure that the right persons have access to only what they need to do their work. This is usualy implemented by systems using some kind of role based access control (in other words being able to grant to a user certain roles granting permissions resources. Kubernetes provides RBAC for this, other cloud providers (GoogleCloud, AWS...) have IAM, and so on so forth.

Now, clearly setting boundaries between what is allowed or not is a difficult exercise (almost impossible?) as we know for sure that human organisations tends to always change. Often those boundaries ends up slowing down your organisation and sometimes becomes a real problem. This is the well known tradeoff between security and usability.

The question is: how do we maintain a decent level of security while also avoiding to  make the life of the system users terrible?

The solution we want to propose here is to allow them to escalate their permission temporarily, under supervision!

Of course escalation isn't something that should be taken lightly, otherwise what's the point of doing access control!

For example we would like to make sure that the following aspects are enforced:

- The user is allowed to escalate
- The escalation is potentially validated by some third party
- The escalation is traced securely for observability.

And this is where Kudo comes into play! This Kubernetes controller aims to;
- Make cluster administrators able to define `EscalationPolicies`. In other words: define who has the right to escalate, what needs to be checked before accepting the escalation and what the escalation actually grants. Policies allows to fine tune permissions escalations to provide the right flexibility while maintaining security.
- Make users able to easily escalate their permissions easily manner, using nothing more that the very well known `kubectl`.

## How it would work?

- Cluster admins define one or many `EscalationPolicy`
- Cluster users create escalations refering a policy. `kubectl create escalation --policy=some-policy-name` (or `kubectl kudo escalate some-policy-name`)
- This creates an Escalation object that is picked up by the kudo-controller. From there the controller will
  - Make sure that the asking user is allowed to use this escalation policy.
  - Run the policies `challenges` to verify at runtime that the user is allowed to perform the escalation.
  - Create the necessary resource(s) to perform the escalation.
  - Whatever the outcome of the escalation attempt, it gets logged into k8s and optionnaly into a secure storage.

### Knowing who is creating the escalation?

We'll actually use an AdmissionWebhook, as the `AdmissionRequest` carries [the necessary user information](https://github.com/kubernetes/kubernetes/blob/master/pkg/apis/admission/types.go#L97).
This webhook will also ask a patch the resource to set the `spec.requestor` field of the escalation.

Some resources about admission webhooks:
- https://medium.com/ovni/writing-a-very-basic-kubernetes-mutating-admission-webhook-398dbbcb63ec

### Properly cleanup created bindings?

That one deserves some thinking. We must absolutely make sure that the controller doesn't link bindings of any kind, that would be a bad bad security flaw.

I was hoping to have some kind of expiration mechanism provided by the API server for bindings, so far I did not find it. Looks like [this is not going to be done](https://github.com/kubernetes/kubernetes/issues/87433#issuecomment-582618496)

After more discussion, we're going to closely look into that strategy:

- Use a [finalizer](https://kubernetes.io/docs/concepts/overview/working-with-objects/finalizers/) to expire all the escalations and destroy all bindings managed by kudo.
- Use the informer resync to check if the escalation and the binding status needs to be updated (that would be the reconciliation loop for bindings)
- Periodicaly scan through all owned resources and see if they need to be reclaimed or not (this would be the garbage collection loop)
- Proooobably understand [this](https://kubernetes.io/docs/concepts/architecture/garbage-collection/#owners-dependents) as well.

Kudos to @jeepsers for the insights!

#### Other ideas

- Use a reclaim cronjob? I think I like that one better because I don't have to deal with who's in charge when running a multi controller setup. Would that be less secure? What if the reclaim job can't be scheduled. bad!

### How to setup and declare CRDs ?

- https://vivilearns2code.github.io/k8s/2021/03/11/writing-controllers-for-kubernetes-custom-resources.html

## CRDs

### EscalationPolicy

An escalation policy define a possible path to escalation. It is composed by the following sections:

- `subjects`: list of principals allowed to use the policy. Type is `[]rbac.authorization.k8s.io.Subject` 
- `challenges`: list of runtime challenges to run when an user attempts to escalate.
- `targets`: list of actions to take to properly implement the escalation.


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
  targets: # (required) what the escalation grants
    - kind: KubernetesRoleBinding
      duration: 60m
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
  targetRef:
    kind: ClusterRoleBinding
    name: rbac-escalation-example/binding-343df
    apiGroup: rbac.authorization.k8s.io
```

### EscalationReview (todo)

```yaml
---
apiVersion: k8s.kudo.dev/v1alpha1
kind: EscalationReview
metadata:
  name: escalation-approval-cdeddef
spec:
  escalationName: escalation-abbdfff3
  reviewerEmail: user-1@kubecluster.com
  status: "APPROVED"
  reviewMessage: "OK"
```

## Escalation challenge ideas

- `PeerReview`: asks for the approval of a third party user.
  - Escalation goes to pending
  - `kubectl kudo approve escalation-abbdfff3 --message "LGTM"`, creates an escalation review
  - When all asked reviews are created with status APPROVED, escalation
- `TimeWindow`: only allow escalation during a certain period of time

## Some more things to figure out

- Notifications ?
- Long term storage
- Resource cleanup strategy?
