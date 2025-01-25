platform ?= $(shell uname -s)
arch     ?= $(shell uname -m)
suffix   ?= $(platform)-$(arch)

BUILD_DIR ?= $(PWD)/build

VKG_BINARY ?= $(BUILD_DIR)/vkg-$(suffix)

.PHONY: debug logs builddir vkg

debug:
	@echo BUILD_DIR  = $(BUILD_DIR)
	@echo VKG_BINARY = $(VKG_BINARY)

logs:
	tail -f matchfile.log | jq 

builddir:
	mkdir -p $(BUILD_DIR)

vkg: debug builddir
	cd cmd/vkg && go build -o $(VKG_BINARY)

runserver: debug vkg
	$(VKG_BINARY) server

runclient: debug vkg
	$(VKG_BINARY) client
