#!/bin/bash

mkdir /tmp/kudo-dev
pushd /tmp/kudo-dev

openssl genrsa -out kudo-test-user.key 2048
openssl req -new -key kudo-test-user.key -out kudo-test-user.csr -subj "/CN=kudo-test-user/O=kudo-test-group"

cat <<EOF | kubectl apply -f -
apiVersion: certificates.k8s.io/v1
kind: CertificateSigningRequest
metadata:
  name: kudo-test-user
spec:
  request: $(cat ./kudo-test-user.csr | base64)
  signerName: kubernetes.io/kube-apiserver-client
  expirationSeconds: 86400
  usages:
  - client auth
EOF

kubectl certificate approve kudo-test-user
kubectl get csr kudo-test-user -o jsonpath='{.status.certificate}'| base64 -d > kudo-test-user.crt

kubectl \
	config set-credentials kudo-test-user \
	--embed-certs=true \
	--client-certificate=./kudo-test-user.crt \
	--client-key=./kudo-test-user.key
kubectl \
  config set-context kudo-test-user \
  --cluster=k3d-kudo-dev \
	--user=kudo-test-user
popd

rm -rf /tmp/kudo-dev
