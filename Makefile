BIN?=bin
NAME?=distributed-algorithms
APP?=${BIN}/${NAME}
PROJECT?=gitlab.mobile-intra.com/cloud/ops/distributed-algorithms
RELEASE?=$(shell cat VERSION.txt)
ifeq ($(IS_CI),)
IS_CI := "false"
NAME := distributed-algorithms
else
IS_CI := "true"
NAME := distributed-algorithms-${JOB_ID}
endif
COMMIT?=$(shell git rev-parse --short HEAD)
BUILD_TIME?=$(shell date -u '+%Y-%m-%d_%H:%M:%S')
LDFLAGS?="-s -w -X ${PROJECT}/version.Release=${RELEASE} -X ${PROJECT}/version.Commit=${COMMIT} -X ${PROJECT}/version.BuildTime=${BUILD_TIME}"
$(info $$IS_CI is ${IS_CI})
DOCKER_IMG?=docker.munic.io/cloud/ops/distributed-algorithms:${RELEASE}

docker-build:
	docker build --build-arg IS_CI=${IS_CI} --build-arg LDFLAGS=${LDFLAGS} -t ${DOCKER_IMG} .

docker-stop:
	docker stop ${NAME} || true
	docker rm -f ${NAME} || true

# docker-test: docker-stop
# 	docker run -v ${PWD}/coverage:/app/coverage --name ${NAME} ${DOCKER_IMG} /bin/sh -c '\
# 	apk add build-base;\
# 	apk add bash git openssh; \
# 	go install github.com/onsi/ginkgo/v2/ginkgo@v2.4.0; \
# 	ginkgo -r --randomize-suites --fail-on-pending --cover --trace --progress --covermode atomic --junit-report=report.xml; \
# 	go tool cover -html=coverprofile.out -o coverage/index.html; \
# 	go tool cover -func=coverprofile.out;\
# 	cp report.xml /app/coverage;'
test:
	ginkgo -r --randomize-suites --fail-on-pending --cover --trace --progress --covermode atomic --junit-report=report.xml
	go tool cover -html=coverprofile.out -o coverage/index.html
	go tool cover -func=coverprofile.out

docker-push:
	docker push ${DOCKER_IMG}
    