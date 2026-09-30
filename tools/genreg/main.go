// Command genreg regenerates the snapshot type registries: go run ./tools/genreg <package dir>...
package main

import (
	"fmt"
	"os"

	"aago/internal/genreg"
)

func main() {
	for _, dir := range os.Args[1:] {
		if err := genreg.Write(dir); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
	}
}
