package oscmd

import (
	"sigs.k8s.io/descheduler/tools/junitreport/pkg/builder"
	"sigs.k8s.io/descheduler/tools/junitreport/pkg/parser"
	"sigs.k8s.io/descheduler/tools/junitreport/pkg/parser/stack"
)

// NewParser returns a new parser that's capable of parsing `os::cmd` test output
func NewParser(builder builder.TestSuitesBuilder, stream bool) parser.TestOutputParser {
	return stack.NewParser(builder, newTestDataParser(), newTestSuiteDataParser(), stream)
}
