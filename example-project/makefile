SHELL := /bin/bash

all: keys web-api metrics tracer

keys:
	go run ./cmd/sales-admin/main.go --cmd keygen

admin:
	go run ./cmd/sales-admin/main.go --cmd useradd --user_email admin@example.com --user_password gophers

web-api:
	docker build \
		-t gcr.io/web-api/web-api-amd64:1.0 \
		--build-arg PACKAGE_NAME=web-api \
		--build-arg VCS_REF=`git rev-parse HEAD` \
		--build-arg BUILD_DATE=`date -u +”%Y-%m-%dT%H:%M:%SZ”` \
		.
	docker system prune -f

metrics:
	docker build \
		-t gcr.io/web-api/metrics-amd64:1.0 \
		--build-arg PACKAGE_NAME=metrics \
		--build-arg PACKAGE_PREFIX=sidecar/ \
		--build-arg VCS_REF=`git rev-parse HEAD` \
		--build-arg BUILD_DATE=`date -u +”%Y-%m-%dT%H:%M:%SZ”` \
		.
	docker system prune -f

tracer:
	docker build \
		-t gcr.io/web-api/tracer-amd64:1.0 \
		--build-arg PACKAGE_NAME=tracer \
		--build-arg PACKAGE_PREFIX=sidecar/ \
		--build-arg VCS_REF=`git rev-parse HEAD` \
		--build-arg BUILD_DATE=`date -u +”%Y-%m-%dT%H:%M:%SZ”` \
		.
	docker system prune -f

up:
	docker-compose up

down:
	docker-compose down

test:
	cd "$$GOPATH/src/geeks-accelerator/oss/saas-starter-kit/example-project"
	go test ./...

clean:
	docker system prune -f

stop-all:
	docker stop $(docker ps -aq)

remove-all:
	docker rm $(docker ps -aq)

#===============================================================================
# GKE

config:
	@echo Setting environment for web-api
	gcloud config set project web-api
	gcloud config set compute/zone us-central1-b
	gcloud auth configure-docker
	@echo ======================================================================

project:
	gcloud projects create web-api
	gcloud beta billing projects link web-api --billing-account=$(ACCOUNT_ID)
	gcloud services enable container.googleapis.com
	@echo ======================================================================

cluster:
	gcloud container clusters create web-api-cluster --num-nodes=2 --machine-type=n1-standard-2
	gcloud compute instances list
	@echo ======================================================================

upload:
	docker push gcr.io/web-api/web-api-amd64:1.0
	docker push gcr.io/web-api/metrics-amd64:1.0
	docker push gcr.io/web-api/tracer-amd64:1.0
	@echo ======================================================================

database:
	kubectl create -f gke-deploy-database.yaml
	kubectl expose -f gke-expose-database.yaml --type=LoadBalancer
	@echo ======================================================================

services:
	kubectl create -f gke-deploy-web-api.yaml
	kubectl expose -f gke-expose-web-api.yaml --type=LoadBalancer
	@echo ======================================================================

shell:
	kubectl exec -it pod-name --container name -- /bin/bash
	@echo ======================================================================

status:
	gcloud container clusters list
	kubectl get nodes
	kubectl get pods
	kubectl get services web-api
	@echo ======================================================================

delete:
	kubectl delete services web-api
	kubectl delete deployment web-api	
	gcloud container clusters delete web-api-cluster
	gcloud projects delete web-api
	docker image remove gcr.io/web-api/web-api-amd64:1.0
	docker image remove gcr.io/web-api/metrics-amd64:1.0
	docker image remove gcr.io/web-api/tracer-amd64:1.0
	@echo ======================================================================
