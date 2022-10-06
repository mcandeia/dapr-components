#
# Copyright 2022 @mcandeia
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#     http://www.apache.org/licenses/LICENSE-2.0
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.
#

################################################################################
# Variables                                                                    #
################################################################################

export GO111MODULE ?= on

COMPONENTS=jsonlogic

COMPONENTS_FOLDER=./components

define build-plugin-target
.PHONY: build-plugin-$(1)
build-plugin-$(1):
	cd $(1); go build -o ../$(COMPONENTS_FOLDER)/$(1).so -buildmode=plugin .; cd -
endef

$(foreach COMPONENT,$(COMPONENTS),$(eval $(call build-plugin-target,$(COMPONENT))))

# Enumerate all generated modtidy targets
BUILD_COMPONENTS:=$(foreach ITEM,$(COMPONENTS),build-plugin-$(ITEM))

.PHONY: build-all
build-all: $(BUILD_COMPONENTS)

$(shell mkdir -p $(COMPONENTS_FOLDER))
