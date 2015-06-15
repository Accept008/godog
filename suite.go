package godog

import (
	"flag"
	"fmt"
	"regexp"
	"strconv"

	"github.com/DATA-DOG/godog/gherkin"
)

// Arg is an argument for StepHandler parsed from
// the regexp submatch to handle the step
type Arg string

// Float converts an argument to float64
// or panics if unable to convert it
func (a Arg) Float() float64 {
	v, err := strconv.ParseFloat(string(a), 64)
	if err == nil {
		return v
	}
	panic(fmt.Sprintf(`cannot convert "%s" to float64: %s`, a, err))
}

// Int converts an argument to int64
// or panics if unable to convert it
func (a Arg) Int() int64 {
	v, err := strconv.ParseInt(string(a), 10, 0)
	if err == nil {
		return v
	}
	panic(fmt.Sprintf(`cannot convert "%s" to int64: %s`, a, err))
}

// String converts an argument to string
func (a Arg) String() string {
	return string(a)
}

// Objects implementing the StepHandler interface can be
// registered as step definitions in godog
//
// HandleStep method receives all arguments which
// will be matched according to the regular expression
// which is passed with a step registration.
// The error in return - represents a reason of failure.
//
// Returning signals that the step has finished
// and that the feature runner can move on to the next
// step.
type StepHandler interface {
	HandleStep(args ...Arg) error
}

// StepHandlerFunc type is an adapter to allow the use of
// ordinary functions as Step handlers.  If f is a function
// with the appropriate signature, StepHandlerFunc(f) is a
// StepHandler object that calls f.
type StepHandlerFunc func(...Arg) error

// HandleStep calls f(step_arguments...).
func (f StepHandlerFunc) HandleStep(args ...Arg) error {
	return f(args...)
}

var errPending = fmt.Errorf("pending step")

type stepMatchHandler struct {
	handler StepHandler
	expr    *regexp.Regexp
}

// Suite is an interface which allows various contexts
// to register step definitions and event handlers
type Suite interface {
	Step(exp *regexp.Regexp, h StepHandler)
}

type suite struct {
	steps    []*stepMatchHandler
	features []*gherkin.Feature
	fmt      Formatter
}

// New initializes a suite which supports the Suite
// interface. The instance is passed around to all
// context initialization functions from *_test.go files
func New() *suite {
	return &suite{}
}

// Step allows to register a StepHandler in Godog
// feature suite, the handler will be applied to all
// steps matching the given regexp
//
// Note that if there are two handlers which may match
// the same step, then the only first matched handler
// will be applied
//
// If none of the StepHandlers are matched, then a pending
// step error will be raised.
func (s *suite) Step(expr *regexp.Regexp, h StepHandler) {
	s.steps = append(s.steps, &stepMatchHandler{
		handler: h,
		expr:    expr,
	})
}

// Run - runs a godog feature suite
func (s *suite) Run() {
	var err error
	if !flag.Parsed() {
		flag.Parse()
	}
	fatal(cfg.validate())

	s.fmt = cfg.formatter()
	s.features, err = cfg.features()
	fatal(err)

	fmt.Println("running", cl("godog", cyan)+", num registered steps:", cl(len(s.steps), yellow))
	fmt.Println("have loaded", cl(len(s.features), yellow), "features from path:", cl(cfg.featuresPath, green))

	for _, f := range s.features {
		s.runFeature(f)
	}
}

func (s *suite) runStep(step *gherkin.Step) (err error) {
	var match *stepMatchHandler
	var args []Arg
	for _, h := range s.steps {
		if m := h.expr.FindStringSubmatch(step.Text); len(m) > 0 {
			match = h
			for _, a := range m[1:] {
				args = append(args, Arg(a))
			}
			break
		}
	}
	if match == nil {
		s.fmt.Pending(step)
		return errPending
	}

	defer func() {
		if e := recover(); e != nil {
			err = e.(error)
			s.fmt.Failed(step, match, err)
		}
	}()

	if err = match.handler.HandleStep(args...); err != nil {
		s.fmt.Failed(step, match, err)
	} else {
		s.fmt.Passed(step, match)
	}
	return
}

func (s *suite) runSteps(steps []*gherkin.Step) bool {
	var failed bool
	for _, step := range steps {
		if failed {
			s.fmt.Skipped(step)
			continue
		}
		if err := s.runStep(step); err != nil {
			failed = true
		}
	}
	return failed
}

func (s *suite) skipSteps(steps []*gherkin.Step) {
	for _, step := range steps {
		s.fmt.Skipped(step)
	}
}

func (s *suite) runFeature(f *gherkin.Feature) {
	s.fmt.Node(f)
	var failed bool
	for _, scenario := range f.Scenarios {
		// background
		// @TODO: do not print more than once
		if f.Background != nil && !failed {
			s.fmt.Node(f.Background)
			failed = s.runSteps(f.Background.Steps)
		}

		// scenario
		s.fmt.Node(scenario)
		if failed {
			s.skipSteps(scenario.Steps)
		} else {
			s.runSteps(scenario.Steps)
		}
	}
}