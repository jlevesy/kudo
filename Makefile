CODE_GENERATOR_VERSION=0.24.3
COUNT?=1
T?=""

K3S_VERSION?=v1.25.0-k3s1

##@ Build and Dependencies

.PHONY: install_dependencies
install_dependencies: ## Install dependencies and code generator
	go get ./...
	go install k8s.io/code-generator/cmd/...@v$(CODE_GENERATOR_VERSION)

.PHONY: unit_tests
unit_tests: ## Run the unit test suite
	go test -failfast -count=$(COUNT) -cover $(shell go list ./... | grep -v e2e)

.PHONY: e2e_tests
e2e_tests: ## Run the end to end test suite
	K3S_VERSION=$(K3S_VERSION) go test -failfast -count=$(COUNT) -run=$(T) -v ./e2e

.PHONY: codegen
codegen: ## Run code generation for CRDs
	@bash ${GOPATH}/pkg/mod/k8s.io/code-generator@v$(CODE_GENERATOR_VERSION)/generate-groups.sh \
		all \
		github.com/jlevesy/kudo/pkg/generated \
		github.com/jlevesy/kudo/pkg/apis \
		k8s.kudo.dev:v1alpha1 \
		--go-header-file ./hack/boilerplate.go.txt

.PHONY: check_codegen
check_codegen: codegen ## Check that no codegen is required
	@git diff --exit-code

##@ Documentation

.PHONY: serve_docs
serve_docs: check_hugo ## Serve the documentation locally.
	hugo server -s ./docs

.PHONY: check_hugo ## Checks that hugo is installed.
check_hugo:
	@hugo version >/dev/null 2>&1 || (echo "ERROR: hugo is required."; exit 1)

##@ Development Environment (main commands, use them)

.PHONY: run_dev
run_dev: preflight_check_dev create_cluster_dev deploy_dev create_test_user_dev wait_controller_ready_dev deploy_environment_resources_dev install_kubectl_plugin_dev ## Runs the development envionment

.PHONY: stop_dev
stop_dev: delete_cluster_dev delete_test_user_dev ## Stop and delete the development envionment

.PHONY: logs_dev
logs_dev: ## Show the controller logs in dev
	kubectl logs -f -l app.kubernetes.io/name=kudo

.PHONY: escalate_dev
escalate_dev: use_test_user_dev apply_escalation_dev use_admin_user_dev ## Simulate an escalation and switch back to the admin user.

.PHONY: apply_escalation_dev

##@ Development Environment (secondary commands)

.PHONY: run_controller_local
run_controller_local: ## Run the controller locally
	go run ./cmd/controller -kubeconfig=${HOME}/.kube/config

.PHONY: debug_controller_local
debug_controller_local: ## Debug the controller using delve.
	dlv debug ./cmd/controller -- -kubeconfig=${HOME}/.kube/config

.PHONY: wait_controller_ready_dev
wait_controller_ready_dev: ## Wait for the controller deployment to be ready
	kubectl rollout status deployment kudo-dev --timeout=90s

.PHONY: deploy_crds_dev
deploy_crds_dev: ## Deploy the kudo CRDs in dev cluster
	kubectl apply -f ./helm/crds

.PHONY: deploy_environment_resources_dev
deploy_environment_resources_dev: ## Deploy examples resources in dev cluster
	kubectl apply -f ./examples/resources

.PHONY: deploy_dev
deploy_dev: deploy_crds_dev ## Build and Deploy kudo in the dev cluster
	helm template \
		--values helm/values.yaml \
		--set image.devRef=ko://github.com/jlevesy/kudo/cmd/controller \
		kudo-dev ./helm | KO_DOCKER_REPO=kudo-registry.localhost:5000 ko apply -B -t dev -f -

.PHONY: create_cluster_dev
create_cluster_dev: ## Create the dev cluster
	k3d cluster create \
		--image="rancher/k3s:$(K3S_VERSION)" \
		--registry-create=kudo-registry.localhost:0.0.0.0:5000 \
		kudo-dev

.PHONY: delete_cluster_dev
delete_cluster_dev: ## Delete the dev cluster
	k3d cluster delete kudo-dev

.PHONY: create_test_user_dev
create_test_user_dev: ## Create the test user
	./hack/create-test-user.sh

.PHONY: delete_test_user_dev
delete_test_user_dev: ## Delete the test user
	-kubectl config delete-user kudo-test-user
	-kubectl config delete-context kudo-test-user

apply_escalation_dev: ## Run escalation with example policy
	kubectl kudo escalate \
		rbac-escalation-policy-example \
		--reason "Needs access to squad-b namespace to debug my service"

.PHONY: use_admin_user_dev
use_admin_user_dev: ## Switch to the admin user
	kubectl config use-context k3d-kudo-dev

.PHONY: use_test_user_dev
use_test_user_dev: ## Switch to the test user
	kubectl config use-context kudo-test-user

.PHONY: install_kubectl_plugin_dev
install_kubectl_plugin_dev: ## Install the kubectl plugin
	go install ./cmd/kubectl-kudo

.PHONY: preflight_check_dev
preflight_check_dev: ## Checks that all the necesary binaries are present
	@helm version >/dev/null 2>&1 || (echo "ERROR: helm is required."; exit 1)
	@k3d version >/dev/null 2>&1 || (echo "ERROR: k3d is required."; exit 1)
	@kubectl version --client >/dev/null 2>&1 || (echo "ERROR: kubectl is required."; exit 1)
	@ko version >/dev/null 2>&1 || (echo "ERROR: google/ko is required."; exit 1)
	@grep -Fq "kudo-registry.localhost" /etc/hosts || (echo "ERROR: please add the following line `kudo-registry.localhost 127.0.0.1` to your /etc/hosts file"; exit 1)

.PHONY: help
help: ## Display this help.
	@awk 'BEGIN {FS = ":.*##"; printf "\nUsage:\n  make \033[36m<target>\033[0m\n"} /^[a-zA-Z_0-9-]+:.*?##/ { printf "  \033[36m%-15s\033[0m %s\n", $$1, $$2 } /^##@/ { printf "\n\033[1m%s\033[0m\n", substr($$0, 5) } ' $(MAKEFILE_LIST)


