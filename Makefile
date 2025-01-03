platform ?= $(shell uname -s)
arch     ?= $(shell uname -m)
suffix   ?= $(platform)-$(arch)

BUILD_DIR ?= $(PWD)/buildfiles

CLIENT ?= $(BUILD_DIR)/client-$(suffix)

.PHONY: logs builddir client runclient

logs:
	tail -f server/matchfile.log | jq 

builddir:
	mkdir -p $(BUILD_DIR)

client: builddir
	cd client && go build -o $(CLIENT)

runclient: client
	tmux new-session -A -s vanitykeygen $(CLIENT)
