package domain

// AuthAction defines user intents for authentication.
type SelectProviderAction struct {
	ID string
}

func (SelectProviderAction) isAction() {}

type SelectAuthMethodAction struct {
	ID string
}

func (SelectAuthMethodAction) isAction() {}

type SubmitFieldAction struct {
	Value string
}

func (SubmitFieldAction) isAction() {}

type SubmitCredentialAction struct {
	Credential Credential
}

func (SubmitCredentialAction) isAction() {}

type RemoveAuthAction struct {
	ProviderID string
}

func (RemoveAuthAction) isAction() {}

// AuthUIUpdate defines workflow updates for the auth UI.
type AuthMethodEvent struct {
	ProviderID string
	Methods    []AuthMethod
}

func (AuthMethodEvent) isUIUpdate() {}

type CredentialFieldEvent struct {
	Method     AuthMethod
	FieldIndex int
}

func (CredentialFieldEvent) isUIUpdate() {}

type AuthErrorEvent struct {
	Error string
}

func (AuthErrorEvent) isUIUpdate() {}

type EnvVarInstructionEvent struct {
	EnvVars []string
}

func (EnvVarInstructionEvent) isUIUpdate() {}
