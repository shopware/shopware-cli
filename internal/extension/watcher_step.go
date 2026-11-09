package extension

// Preparation steps of the admin and storefront watchers, reported when one fails.
const (
	WatcherStepPluginsJSON  = "plugins_json"
	WatcherStepFeatureDump  = "feature_dump"
	WatcherStepNPMInstall   = "npm_install"
	WatcherStepEntitySchema = "entity_schema"
	WatcherStepThemeSelect  = "theme_select"
	WatcherStepThemeCompile = "theme_compile"
	WatcherStepThemeDump    = "theme_dump"
	WatcherStepProxyEnv     = "proxy_env"
)

// WatcherStepError says which preparation step of a watcher failed.
type WatcherStepError struct {
	Step string
	Err  error
}

func (e *WatcherStepError) Error() string { return e.Err.Error() }

func (e *WatcherStepError) Unwrap() error { return e.Err }

// failedStep records the step on a preparation error; nil stays nil.
func failedStep(step string, err error) error {
	if err == nil {
		return nil
	}
	return &WatcherStepError{Step: step, Err: err}
}
