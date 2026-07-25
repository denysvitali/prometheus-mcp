// Command prometheus-mcp serves the Prometheus HTTP API over the Model Context
// Protocol. All wiring lives in the cmd package.
package main

import "github.com/denysvitali/prometheus-mcp/cmd"

func main() {
	cmd.Execute()
}
