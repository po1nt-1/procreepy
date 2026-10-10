package testkit

import "os"

// UnderWine reports whether this is the windows/amd64 suite running under Wine
// on a Linux host, which ci/windows-wine/run-tests.sh signals with
// PROCREEPY_WINE=1.
//
// Wine exposes the host filesystem through drive Z:, so Unix-only device paths
// and real symlinks stay reachable where native Windows has neither. A few
// tests then run a premise that cannot hold on the platform Wine is standing in
// for, and report a Wine-specific message instead of the Windows one. Skipping
// exactly those beats weakening an assertion that is correct everywhere else.
// Native Windows (GitHub's test:windows) runs them all and is the authority.
func UnderWine() bool { return os.Getenv("PROCREEPY_WINE") == "1" }
