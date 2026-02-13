package icons

import _ "embed"

var (
	//go:embed perfolizer-ui.png
	uiBuildIconPNG []byte

	//go:embed perfolizer-agent.png
	agentBuildIconPNG []byte
)

func UIBuildIconPNG() []byte {
	return uiBuildIconPNG
}

func AgentBuildIconPNG() []byte {
	return agentBuildIconPNG
}
