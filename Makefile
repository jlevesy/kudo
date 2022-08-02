KO_DOCKER_REPO=ghcr.io/jlevesy/kudo

.PHONY: unit_tests
unit_tests:
	go test -short -failfast -cover ./...

.PHONY: codegen_v1alpha1
codegen_v1alpha1:
	@bash ${GOPATH}/pkg/mod/k8s.io/code-generator@v0.24.3/generate-groups.sh \
		all \
		github.com/jlevesy/kudo/pkg/client \
		github.com/jlevesy/kudo/pkg/apis \
		k8s.kudo.dev:v1alpha1

.PHONY: check_codegen
check_codegen: codegen_v1alpha1
	@git diff --exit-code

.PHONY: deploy_dev_crds
deploy_dev_crds:
	kubectl apply -f ./helm/crds

.PHONY: deploy_dev
deploy_dev: deploy_dev_crds
	helm template \
		--values helm/values.yaml \
		--set image.devRef=ko://github.com/jlevesy/kudo/cmd/controller \
		kudo-dev ./helm | KO_DOCKER_REPO=$(KO_DOCKER_REPO) ko apply -B -t dev -f -

.PHONY: create_dev_cluster
create_dev_cluster:
	k3d cluster create \
		--image="rancher/k3s:v1.24.3-k3s1" \
		kudo-dev

.PHONY: delete_dev_cluster
delete_dev_cluster:
	k3d cluster delete kudo-dev
