package symfony

// adminWorkerConfigPath is the dotted path to Shopware's admin worker toggle in
// config/packages (shopware.admin_worker.enable_admin_worker). When it is set to
// false the admin no longer dispatches the message queue from the browser, so a
// dedicated process running messenger:consume is required instead.
const adminWorkerConfigPath = "shopware.admin_worker.enable_admin_worker"

// IsAdminWorkerEnabled reports whether Shopware's admin worker is enabled for the
// given environment. The admin worker is enabled by default, so a project that
// does not configure it at all is treated as enabled. It is only considered
// disabled when enable_admin_worker resolves to a literal false.
func (pc *ProjectConfig) IsAdminWorkerEnabled(environment string) (bool, error) {
	value, ok, err := pc.GetResolvedConfigValue(environment, adminWorkerConfigPath)
	if err != nil {
		return false, err
	}

	if !ok {
		return true, nil
	}

	enabled, ok := value.(bool)
	if !ok {
		return true, nil
	}

	return enabled, nil
}

// IsAdminWorkerEnabledForProject loads the project's config/packages tree and
// reports whether the admin worker is enabled for the given environment. It is
// the convenience entry point for callers that only need this one value and do
// not otherwise hold a ProjectConfig. A project without a readable
// config/packages tree defaults to enabled.
func IsAdminWorkerEnabledForProject(projectRoot, environment string) (bool, error) {
	pc, err := NewProjectConfig(projectRoot)
	if err != nil {
		return false, err
	}

	return pc.IsAdminWorkerEnabled(environment)
}
