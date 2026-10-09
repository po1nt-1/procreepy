package main

import (
	"os"
	"os/exec"
	"strings"
	"testing"
)

// childArgsEnv carries the argv for the re-executed child. When it is set the
// test binary runs main() instead of the suite.
const childArgsEnv = "PROCREEPY_MAIN_ARGS"

// TestMainPropagatesExitCode pins the whole of main(): whatever cli.Run
// returns has to become the process exit status. Observing that needs a real
// process exit, so the test re-executes its own binary and inspects the child.
// The byte-exact spelling of these messages belongs to internal/e2e; here only
// enough is matched to prove the run reached the code it claims to.
func TestMainPropagatesExitCode(t *testing.T) {
	if args, ok := os.LookupEnv(childArgsEnv); ok {
		os.Args = append([]string{"procreepy"}, strings.Fields(args)...)
		main()
		// Unreachable while main ends in os.Exit. Returning instead leaves the
		// child exiting 0 with the suite's own output, which fails the parent
		// below — exactly the report a broken wiring deserves.
		return
	}
	tt := []struct {
		name string
		args string
		code int
		want string
	}{
		{"success", "--version", 0, "procreepy "},
		{"usage_error", "--bogus", 2, "procreepy: error: unrecognized arguments: --bogus"},
	}
	for _, tc := range tt {
		t.Run(tc.name, func(t *testing.T) {
			cmd := exec.Command(os.Args[0], "-test.run=^TestMainPropagatesExitCode$")
			cmd.Env = append(os.Environ(), childArgsEnv+"="+tc.args)
			out, err := cmd.CombinedOutput()
			code := 0
			if err != nil {
				ee, ok := err.(*exec.ExitError)
				if !ok {
					t.Fatalf("run child: %v\noutput:\n%s", err, out)
				}
				code = ee.ExitCode()
			}
			if code != tc.code {
				t.Errorf("exit code = %d, want %d\noutput:\n%s", code, tc.code, out)
			}
			if !strings.Contains(string(out), tc.want) {
				t.Errorf("output does not contain %q:\noutput:\n%s", tc.want, out)
			}
		})
	}
}
