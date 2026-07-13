package help

import gohelp "github.com/DeprecatedLuar/gohelp-luar"

func HandleHelp(args []string) error {
	// Build args for gohelp.Run: prepend "help" if needed
	helpArgs := []string{"help"}
	if len(args) > 0 {
		helpArgs = append(helpArgs, args...)
	}

	gohelp.Run(helpArgs,
		RootPage(),
		SetupPage(),
		ConfigPage(),
		ToolsPage(),
		PromptPage(),
		GatewaysPage(),
		SessionsPage(),
		PrerunPage(),
	)

	return nil
}
