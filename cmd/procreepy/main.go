// procreepy extracts the archived timelapse from .procreate files as
// ready-made MP4 videos.
package main

import (
	"os"

	"procreepy/internal/cli"
)

func main() {
	os.Exit(cli.Run(os.Args[1:]))
}
