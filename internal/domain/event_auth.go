package domain

// Auth flow user actions and workflow→auth-UI updates.

// SelectProviderAction is a user intent to select an LLM provider for authentication.
type SelectProviderAction struct {
	ID string
}

func (SelectProviderAction) isAction() {}

// SelectAuthMethodAction is a user intent to select a specific authentication method for a provider.
type SelectAuthMethodAction struct {
	ID string
}

func (SelectAuthMethodAction) isAction() {}

// SubmitFieldAction is a user intent to submit a value for a single authentication field.
type SubmitFieldAction struct {
	Value string
}

func (SubmitFieldAction) isAction() {}

// SubmitCredentialAction is a user intent to submit a complete set of credentials.
type SubmitCredentialAction struct {
	Credential Credential
}

func (SubmitCredentialAction) isAction() {}

// RemoveAuthAction is a user intent to remove authentication for a provider.
type RemoveAuthAction struct {
	ProviderID string
}

func (RemoveAuthAction) isAction() {}

// AuthMethodEvent signals that the user needs to select an authentication method from the given list.
type AuthMethodEvent struct {
	ProviderID string
	Methods    []AuthMethod
}

func (AuthMethodEvent) isUIUpdate() {}

// CredentialFieldEvent signals that the user needs to provide a value for a specific field in an authentication method.
type CredentialFieldEvent struct {
	Method     AuthMethod
	FieldIndex int
}

func (CredentialFieldEvent) isUIUpdate() {}

// AuthErrorEvent signals an error that occurred during the authentication workflow.
type AuthErrorEvent struct {
	Error string
}

func (AuthErrorEvent) isUIUpdate() {}

// EnvVarInstructionEvent provides instructions on which environment variables are needed for authentication.
type EnvVarInstructionEvent struct {
	EnvVars []string
}

func (EnvVarInstructionEvent) isUIUpdate() {}

// OAuthDeviceFlowEvent provides the verification URI and user code for OAuth Device Flow authentication.
type OAuthDeviceFlowEvent struct {
	VerificationURI string
	UserCode        string
}

func (OAuthDeviceFlowEvent) isUIUpdate() {}
