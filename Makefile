platform ?= $(shell uname -s)
arch     ?= $(shell uname -m)
suffix   ?= $(platform)-$(arch)

BUILD_DIR ?= $(PWD)/build

VKG_BINARY ?= $(BUILD_DIR)/vkg-$(suffix)

.PHONY: logs builddir vkg

logs:
	tail -f matchfile.log | jq 

builddir:
	mkdir -p $(BUILD_DIR)

vkg: builddir
	cd cmd/vkg && go build -o $(VBKG_BINARY)
