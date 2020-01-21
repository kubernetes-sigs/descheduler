package parser

import (
	"bufio"

	"sigs.k8s.io/descheduler/tools/junitreport/pkg/api"
)

// TestOutputParser knows how to parse test output to create a collection of test suites
type TestOutputParser interface {
	Parse(input *bufio.Scanner) (*api.TestSuites, error)
}
