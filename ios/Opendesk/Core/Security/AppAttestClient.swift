import CryptoKit
import DeviceCheck
import Foundation

/// Proves an email sign-in, registration or password reset comes from this
/// app on a genuine device (App Attest), which the server accepts in place of
/// Turnstile (contracts/native.md "App Attest"). One key per install per
/// server; its id lives in the Keychain.
///
/// The simulator has no App Attest: debug builds there sign in with OAuth,
/// or against an instance with Turnstile off (see DEVELOPMENT.md).
actor AppAttestClient {
    enum Action: String, Sendable {
        case login
        case register
        case passwordForgot = "password_forgot"
        case passwordReset = "password_reset"
    }

    enum Failure: LocalizedError {
        case unsupported

        var errorDescription: String? {
            "This device can't verify the app, so email sign-in isn't available here. Use another sign-in option."
        }
    }

    private struct Challenge: Decodable, Sendable { let challenge: String }

    private struct KeyBody: Encodable, Sendable {
        let keyID: String
        let attestation: String
        let challenge: String

        enum CodingKeys: String, CodingKey {
            case keyID = "key_id"
            case attestation, challenge
        }
    }

    private let api: APIClient
    private var registering: Task<String, Error>?

    init(api: APIClient) {
        self.api = api
    }

    nonisolated static var isSupported: Bool { DCAppAttestService.shared.isSupported }

    private var keyAccount: String { "app-attest." + api.instance.storageKey }

    /// Signs a fresh challenge for `action`, registering this install's key first if needed.
    func proof(for action: Action) async throws -> AppAttestProof {
        guard Self.isSupported else { throw Failure.unsupported }
        let keyID = try await registeredKey()
        let challenge = try await fetchChallenge()
        let clientData = Data("opendesk-app-attest/v1\n\(action.rawValue)\n\(challenge)".utf8)
        do {
            let assertion = try await DCAppAttestService.shared.generateAssertion(keyID, clientDataHash: Self.sha256(clientData))
            return AppAttestProof(keyID: keyID, assertion: assertion.base64EncodedString(), challenge: challenge)
        } catch let error as DCError where error.code == .invalidKey {
            // The system dropped the key (a restore onto another device): the next proof makes a new one.
            forgetKey()
            throw error
        }
    }

    /// Drops the stored key, so the next proof registers a new one (also after the server refused it).
    func forgetKey() {
        KeychainStore.remove(keyAccount)
    }

    private func registeredKey() async throws -> String {
        if let keyID = KeychainStore.get(keyAccount) { return keyID }
        if let registering { return try await registering.value }
        let task = Task { try await self.register() }
        registering = task
        defer { registering = nil }
        let keyID = try await task.value
        KeychainStore.set(keyID, for: keyAccount)
        return keyID
    }

    private func register() async throws -> String {
        let service = DCAppAttestService.shared
        let keyID = try await service.generateKey()
        let challenge = try await fetchChallenge()
        let attestation = try await service.attestKey(keyID, clientDataHash: Self.sha256(Data(challenge.utf8)))
        let body = KeyBody(keyID: keyID, attestation: attestation.base64EncodedString(), challenge: challenge)
        let _: EmptyBody = try await api.post("v1/auth/app-attest/keys", body: body, workspace: .none, authorized: false)
        return keyID
    }

    private func fetchChallenge() async throws -> String {
        let response: Challenge = try await api.post("v1/auth/app-attest/challenge", workspace: .none, authorized: false)
        return response.challenge
    }

    private static func sha256(_ data: Data) -> Data {
        Data(SHA256.hash(data: data))
    }
}

/// PKCE for the native OAuth flow: a 64-character verifier and its S256 challenge.
enum PKCE {
    static func verifier() -> String {
        var bytes = [UInt8](repeating: 0, count: 48)
        _ = SecRandomCopyBytes(kSecRandomDefault, bytes.count, &bytes)
        return base64URL(Data(bytes))
    }

    static func challenge(for verifier: String) -> String {
        base64URL(Data(SHA256.hash(data: Data(verifier.utf8))))
    }

    static func base64URL(_ data: Data) -> String {
        data.base64EncodedString()
            .replacingOccurrences(of: "+", with: "-")
            .replacingOccurrences(of: "/", with: "_")
            .replacingOccurrences(of: "=", with: "")
    }

    static func decodeBase64URL(_ text: String) -> Data? {
        var base = text.replacingOccurrences(of: "-", with: "+").replacingOccurrences(of: "_", with: "/")
        while base.count % 4 != 0 { base += "=" }
        return Data(base64Encoded: base)
    }
}
