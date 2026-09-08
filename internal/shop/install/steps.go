package install

import "strings"

// Step maps a deployment-helper console command to a user-facing label. The
// helper prints "Start: <command> ..." lines; matching them is the only
// progress signal the installation offers.
type Step struct {
	Pattern string
	Label   string
}

// Steps are the recognized installation phases, in execution order.
var Steps = []Step{
	{"system:install", "Installing Shopware"},
	{"user:create", "Creating admin account"},
	{"messenger:setup-transports", "Setting up message transports"},
	{"sales-channel:create:storefront", "Creating storefront"},
	{"theme:change", "Compiling theme"},
	{"plugin:refresh", "Refreshing plugins"},
}

// stepStartPrefix marks deployment-helper lines that announce a new command.
const stepStartPrefix = "Start: "

// MatchStep reports the index of the step a deployment-helper output line
// starts. Progress is monotonic: only steps at or after from match, so a
// pattern echoed later in the output cannot move progress backwards.
func MatchStep(line string, from int) (int, bool) {
	if !strings.HasPrefix(line, stepStartPrefix) {
		return 0, false
	}
	for i, step := range Steps {
		if i >= from && strings.Contains(line, step.Pattern) {
			return i, true
		}
	}
	return 0, false
}

// FailedStepSaveCredentials is the failed_step telemetry value for an
// installation that succeeded but whose admin credentials could not be saved
// to the project config.
const FailedStepSaveCredentials = "save_credentials"

// FailedStep names the last step that had started when the install failed.
// Failures before the first recognized step report the first step.
func FailedStep(current int) string {
	if current >= len(Steps) {
		current = len(Steps) - 1
	}
	return Steps[current].Pattern
}
