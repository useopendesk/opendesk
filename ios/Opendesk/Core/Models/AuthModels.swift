import Foundation

/// `GET <app origin>/config.json`: the same values the web app reads from
/// `/config.js` (contracts/native.md "Finding an instance").
struct RuntimeConfig: Codable, Hashable, Sendable {
    var apiUrl: String
    var gatewayUrl: String?
    var selfHosted: Bool?
    var turnstileSiteKey: String?
}

struct AuthProvider: Codable, Hashable, Sendable, Identifiable {
    var id: String
    var name: String
}

enum PushMode: String, LenientEnum {
    case direct, relay, off
    static let fallback = PushMode.off
}

struct NativeAuthConfig: Codable, Hashable, Sendable {
    var appAttest: Bool
    var push: PushMode
    var relayURL: String?

    enum CodingKeys: String, CodingKey {
        case appAttest = "app_attest"
        case push
        case relayURL = "relay_url"
    }

    static let unavailable = NativeAuthConfig(appAttest: false, push: .off, relayURL: nil)
}

/// `GET /v1/auth/config`.
struct AuthConfig: Codable, Hashable, Sendable {
    var providers: [AuthProvider]
    var passwordLogin: Bool
    var signupEnabled: Bool
    var passkeys: Bool
    var turnstileSiteKey: String?
    var native: NativeAuthConfig

    enum CodingKeys: String, CodingKey {
        case providers, passkeys, native
        case passwordLogin = "password_login"
        case signupEnabled = "signup_enabled"
        case turnstileSiteKey = "turnstile_site_key"
    }

    init(from decoder: Decoder) throws {
        let c = try decoder.container(keyedBy: CodingKeys.self)
        providers = c.lenient([AuthProvider].self, forKey: .providers) ?? []
        passwordLogin = c.lenient(Bool.self, forKey: .passwordLogin) ?? true
        signupEnabled = c.lenient(Bool.self, forKey: .signupEnabled) ?? false
        passkeys = c.lenient(Bool.self, forKey: .passkeys) ?? false
        turnstileSiteKey = c.lenient(String.self, forKey: .turnstileSiteKey)
        // An instance from before native apps has no `native` member: no App Attest, no push.
        native = c.lenient(NativeAuthConfig.self, forKey: .native) ?? .unavailable
    }

    /// The instance puts sign-in behind a human check.
    var humanCheckRequired: Bool { !(turnstileSiteKey ?? "").isEmpty }

    /// Email and password work in the app: no human check, or App Attest stands in for it.
    var emailSignInAvailable: Bool {
        passwordLogin && (!humanCheckRequired || native.appAttest)
    }
}

/// Every answer of the session-issuing endpoints (login, register, mfa,
/// oauth exchange and link, passkey finish, refresh).
struct AuthResult: Decodable, Sendable {
    var accessToken: String?
    var expiresIn: Int?
    var refreshToken: String?
    var user: User?
    var mfaRequired: Bool
    var mfaToken: String?
    var linkRequired: Bool
    var linkToken: String?
    var email: String?
    var redirectTo: String?

    enum CodingKeys: String, CodingKey {
        case user, email
        case accessToken = "access_token"
        case expiresIn = "expires_in"
        case refreshToken = "refresh_token"
        case mfaRequired = "mfa_required"
        case mfaToken = "mfa_token"
        case linkRequired = "link_required"
        case linkToken = "link_token"
        case redirectTo = "redirect_to"
    }

    init(from decoder: Decoder) throws {
        let c = try decoder.container(keyedBy: CodingKeys.self)
        accessToken = c.lenient(String.self, forKey: .accessToken)
        expiresIn = c.lenient(Int.self, forKey: .expiresIn)
        refreshToken = c.lenient(String.self, forKey: .refreshToken)
        user = c.lenient(User.self, forKey: .user)
        mfaRequired = c.lenient(Bool.self, forKey: .mfaRequired) ?? false
        mfaToken = c.lenient(String.self, forKey: .mfaToken)
        linkRequired = c.lenient(Bool.self, forKey: .linkRequired) ?? false
        linkToken = c.lenient(String.self, forKey: .linkToken)
        email = c.lenient(String.self, forKey: .email)
        redirectTo = c.lenient(String.self, forKey: .redirectTo)
    }
}

/// The `app_attest` member of a protected auth request.
struct AppAttestProof: Encodable, Sendable {
    var keyID: String
    var assertion: String
    var challenge: String

    enum CodingKeys: String, CodingKey {
        case keyID = "key_id"
        case assertion, challenge
    }
}

struct PasskeyBegin: Decodable, Sendable {
    var options: JSONValue
    var passkeyToken: String

    enum CodingKeys: String, CodingKey {
        case options
        case passkeyToken = "passkey_token"
    }
}
