package runtime

const (
	// InjectAccountEmail is the email of the simulated account in local mode.
	InjectAccountEmail = "cursor@ai.com"
	// InjectAuthToken is the token of the simulated account in local mode.
	InjectAuthToken = "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJzdWIiOiJmYWtlLWN1cnNvci1sb2NhbC11c2VyIiwiZW1haWwiOiJjdXJzb3JAYWkuY29tIiwidHlwZSI6InNlc3Npb24iLCJpc3MiOiJjdXJzb3ItY2xpZW50Iiwic2NvcGUiOiJvcGVuaWQgcHJvZmlsZSBlbWFpbCIsImV4cCI6NDA3MDkwODgwMH0.fake-local-state-token"
	// LocalRelayToken is used in local mode to override the Authorization header when the backend fetches from cursor.sh as the origin.
	LocalRelayToken = InjectAuthToken
)
