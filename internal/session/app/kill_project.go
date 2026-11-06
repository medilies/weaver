package app

import "github.com/medilies/weaver/internal/session/domain"

// KillProject kills a project's tmux session and everything running in
// it.
type KillProject struct {
	sessions domain.SessionKiller
}

// NewKillProject constructs the use case against its port.
func NewKillProject(sessions domain.SessionKiller) KillProject {
	return KillProject{sessions: sessions}
}

// Handle kills the session of the project named rawName.
func (uc KillProject) Handle(rawName string) error {
	name, err := domain.NewProjectName(rawName)
	if err != nil {
		return err
	}
	return uc.sessions.Kill(name)
}
