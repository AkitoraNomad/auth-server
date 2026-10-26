package internal

const PlaceholderFile = "placeholder.jpg"

const (
	SessionKeyUserID                 = "LoggedInUserID"
	SessionKeyConsentByRequest       = "consentByRequest"
	SessionKeyTelegramData           = "telegramData"
	SessionKeyAuthorizeRequests      = "authorizeRequests"
	SessionKeyActiveAuthorizeRequest = "activeAuthorizeRequest"
)

const (
	PathLogin        = "/login"
	PathAuth         = "/auth"
	PathAuthorize    = "/oauth2/authorize"
	PathToken        = "/oauth2/token"
	PathIntrospect   = "/introspect"
	PathUserinfo     = "/userinfo"
	PathRegistration = "/registration"
	PathDiscovery    = "/.well-known/openid-configuration"
	PathJwkSet       = "/.well-known/jwks.json"
)
