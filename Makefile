platform ?= $(shell uname -s)
arch     ?= $(shell uname -m)
suffix   ?= $(platform)-$(arch)

BUILD_DIR ?= $(PWD)/buildfiles

CLIENT ?= $(BUILD_DIR)/vkg-client-$(suffix)
SERVER ?= $(BUILD_DIR)/vkg-server-$(suffix)


.PHONY: logs builddir client runclient

logs:
	tail -f server/matchfile.log | jq 

builddir:
	mkdir -p $(BUILD_DIR)

client: builddir
	cd client && go build -o $(CLIENT)

runclient: client
	tmux new-session -A -s vkgclient $(CLIENT)

server: builddir
	cd server && go build -o $(SERVER)
	
runserver:
	tmux new-session -A -s vkgserver $(SERVER)
