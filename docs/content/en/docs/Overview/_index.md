---
title: "Overview"
linkTitle: "Overview"
weight: 1
description: >
  Get a quick idea of what is Kudo
---

## What is Kudo?

Kudo is a Kubernetes controller that allows individual users to temporarily escalate their permissions while still maintaining security and crystal clear auditability.

It comes in complement of existing access control systems (Kubernetes RBAC, GCP IAM), and relies on them to temporarily grant or reclaim permissions. In the context of Kubernetes, Kudo temporarily creates a `RoleBinding` or a `ClusterRoleBinding` between an existing role and the escalation requestor.

It is built around two main concepts, defined as Kubernetes Custom Resources:

- *Escalation Policies*: Created by the cluster administrators, escalation policies defines the "rules" of escalations. To be more specific, it defines who has the right to escalate, what needs to be checked when the escalation is created, and what escalating grants.
- *Escalations* are created by users who want to escalate temporarily their permissions and refers to a policy. If the escalation is accepted, kudo grants temporarily new permissions, and reclaims them when the escalation expires.

## Why Allowing Users to Escalate ?

Defining minimal and secure authorization model is a difficult exercise, because it is often hard to predict which permissions people are going to need to actually get their job done. In many situations, this model is limiting and people need to temporarily bypass it.

Let's take an example: What happens when someone asks to temporarily get more permissions? Your administration team grants them after checking if their demand is legit. Then they may or may not reclaim those permissions... depending on if they think about it or not!

At best this is tedious work, at worst this becomes a security issue!

Kudo aims to address this growing pains by automating the process of escalating while still maintaining a high level of security.

In other words, Kudo allows you to:

* **Enforce a minimal set of permissions default**, to implement the principle of least privilege.
* **Provision escalation policies**, to define precisely who has the right to get what.
* **Get your users to easilly escalate their permissions**, without having to ask an administrator, while still maintaining security. No friction for them, less work for your admins!
* **Control who's escalating**, by defining escalation challenges. For example requiring an escalation to be approved by a peer from another team before being granted.
* **Temporarily Grant permissions across systems**: could it be Kubernetes RBAC, AWS or GCP IAM. It all boils done to what the escalation policy defines.
* **Audit escalations**: Automatically record escalation events into third party systems to keep a trail of what happened.

## Where should I go next?

Interested into going deeper? Feel free to check out the following sections:

* [Getting Started](../getting-started): Get started with Kudo
* [Core Concepts](../concepts): Get to know Kudo core concepts
* [Examples](../examples): Check out some example use cases for Kudo!
