PROJECT?=vkg
REGISTRY?=docker.io
LIBRARY?=nugget

image=$(REGISTRY)/$(LIBRARY)/$(PROJECT)

platforms=linux/amd64,linux/arm64

prodtag=latest
devtag=dev
builder=builder-$(PROJECT)

GIT ?= $(shell which git)
PWD ?= $(shell pwd)

platform ?= $(shell uname -s)
arch     ?= $(shell uname -m)
suffix   ?= $(platform)-$(arch)


BUILD_DIR ?= $(PWD)/build

VKG_BINARY ?= $(BUILD_DIR)/vkg-$(suffix)

OCI_IMAGE_CREATED="$(shell date -u +"%Y-%m-%dT%H:%M:%SZ")"
OCI_IMAGE_REVISION?="$(shell $(GIT) rev-parse HEAD)"
OCI_IMAGE_VERSION?="$(shell $(GIT) describe --always --long --tags --dirty)"

oci-build-labels?=\
	--build-arg OCI_IMAGE_CREATED=$(OCI_IMAGE_CREATED) \
	--build-arg OCI_IMAGE_VERSION=$(OCI_IMAGE_VERSION) \
	--build-arg OCI_IMAGE_REVISION=$(OCI_IMAGE_REVISION) 

oci-version-point=$(shell echo $(OCI_IMAGE_VERSION) | cut -f 2 -d v | cut -f 1 -d '-')
oci-version-minor=$(shell echo $(oci-version-point) | cut -f 1-2 -d .)
oci-version-major=$(shell echo $(oci-version-point) | cut -f 1 -d .)

LD_FLAGS="-X 'main.gitVersion=$(OCI_IMAGE_VERSION)'"

.PHONY: debug logs builddir vkg vkgstatic

debug:
	@echo BUILD_DIR  = $(BUILD_DIR)
	@echo VKG_BINARY = $(VKG_BINARY)

clean:
	rm -f vkg-static-build
	-docker buildx rm $(builder)

logs:
	tail -f matchfile.log | jq 

builddir:
	mkdir -p $(BUILD_DIR)

vkg: debug builddir
	cd cmd/vkg && go build -o $(VKG_BINARY)

vkgstatic: debug builddir
	cd cmd/vkg && CGO_ENABLED=0 go build -ldflags=$(LD_FLAGS) -o ../../vkg-static-build .

buildx: debug clean
	docker buildx create --name $(builder)
	docker buildx use $(builder)
	docker buildx install
	@echo

buildserver: image = $(REGISTRY)/$(LIBRARY)/$(PROJECT)-server
buildserver: debug buildx 
	image=vkg-server
	@echo "# making: prod"
	docker buildx use $(builder)
	docker buildx build $(oci-build-labels) \
		-t $(image):$(prodtag) \
		-t $(image):$(oci-version-major) \
		-t $(image):$(oci-version-minor) \
		-t $(image):$(oci-version-point) \
		--platform=$(platforms) --push . 
	docker buildx rm $(builder)
	docker pull $(image):$(prodtag)

buildclient: vkgstatic
	docker build . -t nugget/vkg-client:dev

runserver: debug vkg
	$(VKG_BINARY) server

runclient: debug vkg
	$(VKG_BINARY) client

