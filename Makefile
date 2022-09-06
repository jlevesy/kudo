CODE_GENERATOR_VERSION=0.24.3
COUNT?=1
T?=""

.PHONY: install_dependencies
install_dependencies:
	go get ./...
	go install k8s.io/code-generator/cmd/...@v$(CODE_GENERATOR_VERSION)

.PHONY: unit_tests
unit_tests:
	go test -failfast -count=$(COUNT) -cover $(shell go list ./... | grep -v e2e)

.PHONY: e2e_tests
e2e_tests:
	go test -failfast -count=$(COUNT) -run=$(T) -v ./e2e

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

.PHONY: serve_docs
serve_docs: check_hugo
	hugo server -s ./docs

.PHONY: check_hugo
check_hugo:
	@hugo version >/dev/null 2>&1 || (echo "ERROR: hugo is required."; exit 1)

.PHONY: run_controller_local
run_controller_local:
	go run ./cmd/controller -kubeconfig=${HOME}/.kube/config

.PHONY: debug_controller_local
debug_controller_local:
	dlv debug ./cmd/controller -- -kubeconfig=${HOME}/.kube/config

.PHONY: run_dev
run_dev: preflight_check_dev create_cluster_dev deploy_dev create_test_user_dev wait_controller_ready_dev deploy_environment_resources_dev install_kubectl_escalate_plugin_dev

.PHONY: wait_controller_ready_dev
wait_controller_ready_dev:
	kubectl rollout status deployment kudo-dev --timeout=90s

.PHONY: deploy_crds_dev
deploy_crds_dev:
	kubectl apply -f ./helm/crds

.PHONY: deploy_environment_resources_dev
deploy_environment_resources_dev:
	kubectl apply -f ./examples/resources

.PHONY: deploy_dev
deploy_dev: deploy_crds_dev
	helm template \
		--values helm/values.yaml \
		--set image.devRef=ko://github.com/jlevesy/kudo/cmd/controller \
		kudo-dev ./helm | KO_DOCKER_REPO=kudo-registry.localhost:5000 ko apply -B -t dev -f -

.PHONY: logs_dev
logs_dev:
	kubectl logs -f -l app.kubernetes.io/name=kudo

.PHONY: create_cluster_dev
create_cluster_dev:
	k3d cluster create \
		--image="rancher/k3s:v1.24.3-k3s1" \
		--registry-create=kudo-registry.localhost:0.0.0.0:5000 \
		kudo-dev

.PHONY: delete_cluster_dev
delete_cluster_dev: delete_test_user_dev
	k3d cluster delete kudo-dev

.PHONY: create_test_user_dev
create_test_user_dev:
	./hack/create-test-user.sh

.PHONY: delete_test_user_dev
delete_test_user_dev:
	-kubectl config delete-user kudo-test-user
	-kubectl config delete-context kudo-test-user

.PHONY: run_escalation_dev
run_escalation_dev: use_test_user_dev apply_escalation_dev use_admin_user_dev

.PHONY: apply_escalation_dev
apply_escalation_dev:
	kubectl kudo escalate \
		rbac-escalation-policy-example \
		"Needs access to squad-b namespace to debug my service"

.PHONY: use_admin_user_dev
use_admin_user_dev:
	kubectl config use-context k3d-kudo-dev

.PHONY: use_test_user_dev
use_test_user_dev:
	kubectl config use-context kudo-test-user

.PHONY: install_kubectl_escalate_plugin_dev
install_kubectl_escalate_plugin_dev:
	go install ./cmd/kubectl-kudo-escalate

.PHONY: preflight_check_dev
preflight_check_dev:
	@helm version >/dev/null 2>&1 || (echo "ERROR: helm is required."; exit 1)
	@k3d version >/dev/null 2>&1 || (echo "ERROR: k3d is required."; exit 1)
	@kubectl version --client >/dev/null 2>&1 || (echo "ERROR: kubectl is required."; exit 1)
	@ko version >/dev/null 2>&1 || (echo "ERROR: google/ko is required."; exit 1)
	@grep -Fq "kudo-registry.localhost" /etc/hosts || (echo "ERROR: please add the following line `kudo-registry.localhost 127.0.0.1` to your /etc/hosts file"; exit 1)
