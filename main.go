/*
Copyright 2022 @mcandeia
Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at
    http://www.apache.org/licenses/LICENSE-2.0
Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package main

import (
	"os"
	"path/filepath"
	"plugin"

	dapr "github.com/dapr-sandbox/components-go-sdk"
	"github.com/pkg/errors"
)

const (
	pluginsDefaultFolder = "./components"
	pluginsEnvVar        = "DAPR_COMPONENTS_PLUGINS_FOLDER"
)

// getPluginsFolder returns the shared unix domain socket folder path
func getPluginsFolder() string {
	if f, ok := os.LookupEnv(pluginsEnvVar); ok {
		return f
	}
	return pluginsDefaultFolder
}

// openPlugins open all component plugins applying side effect and registering then into the dapr register.
func openPlugins() error {
	pluginsFolder := getPluginsFolder()
	_, err := os.Stat(pluginsFolder)

	if os.IsNotExist(err) { // not exists is the same as empty.
		return nil
	}

	plugins, err := os.ReadDir(pluginsFolder)
	if err != nil {
		return errors.Wrap(err, "could not list plugins")
	}
	for _, plg := range plugins {
		if plg.IsDir() {
			continue
		}
		_, err := plugin.Open(filepath.Join(pluginsFolder, plg.Name()))
		if err != nil {
			return err
		}
	}
	return nil
}

func main() {
	if err := openPlugins(); err != nil {
		panic(err)
	}

	dapr.MustRun()
}
