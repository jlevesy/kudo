---
categories: ["Guide", "Getting Started", "Installation"]
tags: ["installation","getting-started"]
title: "Getting Started"
linkTitle: "Getting Started"
weight: 2
description: >
  Get started with Kudo in a Few easy Steps
---

## Prerequisites

- Kubernetes 1.24 (earlier versions might work but we haven't tested it yet).
- Helm 3

## Installation

Kudo has no release yet so we can't properly document this.

### Installing the Controller

This will probably look like `helm install -f values.yaml -n kudo kudo/kudo-controller kudo-controller`

We need to mention here:

- A cert is generated in the helm chart, so people need to be cautious about this. We also need to provide a way of refreshing the cert (I'm thinking helm template only the cert file, but this needs to be tested.)

### Installing the kubectl Plugin

I would say `kubectl krew install kudo`  but only future will tell.

## Setup

Refers to the [examples page](../examples) to get examples.
