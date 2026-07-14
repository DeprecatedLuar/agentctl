package templates

import _ "embed"

//go:embed files/config/agent.toml
var AgentToml string

//go:embed files/.data/contacts.toml
var ContactsToml string

//go:embed files/prompts/chat_template
var Prompt string

//go:embed files/prompts/system.md
var SystemMd string

//go:embed files/tool-example.toml
var ToolExample string

//go:embed files/routine-example.toml
var RoutineExample string

//go:embed files/env.template
var EnvTemplate string

//go:embed files/gitignore
var Gitignore string

//go:embed files/.prerun.sh
var PrerunSh string
