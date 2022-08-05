KO_DOCKER_REPO=ghcr.io/jlevesy/kudo
CODE_GENERATOR_VERSION=0.24.3

.PHONY: install_dependencies
install_dependencies:
	go get ./...
	go install k8s.io/code-generator/cmd/...@v$(CODE_GENERATOR_VERSION)

.PHONY: unit_tests
unit_tests:
	go test -short -failfast -cover ./...

.PHONY: codegen
codegen:
	@bash ${GOPATH}/pkg/mod/k8s.io/code-generator@v$(CODE_GENERATOR_VERSION)/generate-groups.sh \
		all \
		github.com/jlevesy/kudo/pkg/generated \
		github.com/jlevesy/kudo/pkg/apis \
		k8s.kudo.dev:v1alpha1 \
		--go-header-file ./hack/boilerplate.go.txt

.PHONY: check_codegen
check_codegen: codegen
	@git diff --exit-code

.PHONY: run_controller_local
run_controller_local:
	go run ./cmd/controller -kubeconfig=${HOME}/.kube/config

.PHONY: deploy_crds_dev
deploy_crds_dev:
	kubectl apply -f ./helm/crds

.PHONY: deploy_dev
deploy_dev: deploy_crds_dev
	helm template \
		--values helm/values.yaml \
		--set image.devRef=ko://github.com/jlevesy/kudo/cmd/controller \
		kudo-dev ./helm | KO_DOCKER_REPO=$(KO_DOCKER_REPO) ko apply -B -t dev -f -

.PHONY: logs_dev
logs_dev:
	kubectl logs -f -l app.kubernetes.io/name=kudo

.PHONY: create_cluster_dev
create_cluster_dev:
	k3d cluster create \
		--image="rancher/k3s:v1.24.3-k3s1" \
		kudo-dev

.PHONY: delete_cluster_dev
delete_cluster_dev:
	k3d cluster delete kudo-dev
