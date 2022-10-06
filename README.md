# dapr-components

Personal [Dapr](http://github.com/dapr/dapr) components repository made using Pluggable Components SDKs.

## How it works

This project uses [Go Plugins](https://pkg.go.dev/plugin), it compiles the components as a plugin and place them in a common folder using the plugin name. Let's say you want to use the `jsonlogic` plugin, then you should compile it by using `make build-all` and then after you can run using `main.go`.

You can select a subset of all components by just providing `COMPONENTS=` environment variable.

```shell
COMPONENTS=jsonlogic aws-qldb make build-all
```
