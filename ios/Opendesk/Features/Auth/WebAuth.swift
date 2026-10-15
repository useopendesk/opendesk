import AuthenticationServices
import Foundation
import UIKit

/// The key window, for system sheets that need an anchor.
@MainActor
enum PresentationAnchor {
    static var window: UIWindow {
        let scenes = UIApplication.shared.connectedScenes.compactMap { $0 as? UIWindowScene }
        if let window = scenes.flatMap(\.windows).first(where: \.isKeyWindow) { return window }
        if let window = scenes.flatMap(\.windows).first { return window }
        return ASPresentationAnchor()
    }
}

/// Native OAuth: the provider runs in `ASWebAuthenticationSession`, and the
/// server sends the result to `opendesk://auth/callback?code=` (contracts/native.md).
@MainActor
final class OAuthSignIn: NSObject, ASWebAuthenticationPresentationContextProviding {
    enum Failure: LocalizedError {
        case cancelled
        case provider(String)
        case noCode

        var errorDescription: String? {
            switch self {
            case .cancelled: nil
            case let .provider(code):
                switch code {
                case "access_denied": "Sign-in was cancelled at the provider."
                case "email_unverified": "That account's email isn't verified with the provider."
                case "signup_disabled": "This server only accepts invited people."
                case "account_disabled": "That account can't sign in."
                default: "The provider didn't complete the sign-in. Try again."
                }
            case .noCode: "The sign-in didn't finish. Try again."
            }
        }
    }

    private var session: ASWebAuthenticationSession?

    /// Runs the flow and returns the one-time code.
    func run(startURL: URL) async throws -> String {
        let callback: URL = try await withCheckedThrowingContinuation { continuation in
            let session = ASWebAuthenticationSession(url: startURL, callback: .customScheme("opendesk")) { url, error in
                if let error = error as? ASWebAuthenticationSessionError, error.code == .canceledLogin {
                    continuation.resume(throwing: Failure.cancelled)
                } else if let error {
                    continuation.resume(throwing: error)
                } else if let url {
                    continuation.resume(returning: url)
                } else {
                    continuation.resume(throwing: Failure.noCode)
                }
            }
            session.presentationContextProvider = self
            // A shared session would reuse the browser's cookies; each sign-in starts clean.
            session.prefersEphemeralWebBrowserSession = true
            self.session = session
            if !session.start() { continuation.resume(throwing: Failure.noCode) }
        }
        session = nil
        let items = URLComponents(url: callback, resolvingAgainstBaseURL: false)?.queryItems ?? []
        if let error = items.first(where: { $0.name == "error" })?.value { throw Failure.provider(error) }
        guard let code = items.first(where: { $0.name == "code" })?.value, !code.isEmpty else { throw Failure.noCode }
        return code
    }

    nonisolated func presentationAnchor(for session: ASWebAuthenticationSession) -> ASPresentationAnchor {
        MainActor.assumeIsolated { PresentationAnchor.window }
    }
}

/// Passkey sign-in with the platform authenticator. Offered only on Cloud,
/// where the app is associated with the RP host (`webcredentials:app.opendesk.sh`).
@MainActor
final class PasskeySignIn: NSObject, ASAuthorizationControllerDelegate, ASAuthorizationControllerPresentationContextProviding {
    enum Failure: LocalizedError {
        case cancelled
        case badOptions
        case failed

        var errorDescription: String? {
            switch self {
            case .cancelled: nil
            case .badOptions: "The server sent passkey options the app can't use."
            case .failed: "That passkey didn't work. Try again or use another sign-in option."
            }
        }
    }

    private var continuation: CheckedContinuation<ASAuthorizationPlatformPublicKeyCredentialAssertion, Error>?
    private var controller: ASAuthorizationController?

    /// Asserts with a passkey for `options` (WebAuthn `PublicKeyCredentialRequestOptionsJSON`)
    /// and returns the `AuthenticationResponseJSON` the server expects.
    func assert(options raw: JSONValue, fallbackRP: String) async throws -> JSONValue {
        let options = raw["publicKey"] ?? raw
        guard let challengeText = options["challenge"]?.string, let challenge = PKCE.decodeBase64URL(challengeText) else {
            throw Failure.badOptions
        }
        let rp = options["rpId"]?.string ?? fallbackRP
        let provider = ASAuthorizationPlatformPublicKeyCredentialProvider(relyingPartyIdentifier: rp)
        let request = provider.createCredentialAssertionRequest(challenge: challenge)
        switch options["userVerification"]?.string {
        case "required": request.userVerificationPreference = .required
        case "discouraged": request.userVerificationPreference = .discouraged
        default: request.userVerificationPreference = .preferred
        }
        if let allowed = options["allowCredentials"]?.array, !allowed.isEmpty {
            request.allowedCredentials = allowed.compactMap { $0["id"]?.string.flatMap(PKCE.decodeBase64URL) }
                .map { ASAuthorizationPlatformPublicKeyCredentialDescriptor(credentialID: $0) }
        }

        let assertion = try await withCheckedThrowingContinuation { continuation in
            self.continuation = continuation
            let controller = ASAuthorizationController(authorizationRequests: [request])
            controller.delegate = self
            controller.presentationContextProvider = self
            self.controller = controller
            controller.performRequests()
        }
        controller = nil

        let id = PKCE.base64URL(assertion.credentialID)
        var response: [String: JSONValue] = [
            "clientDataJSON": .string(PKCE.base64URL(assertion.rawClientDataJSON)),
            "authenticatorData": .string(PKCE.base64URL(assertion.rawAuthenticatorData)),
            "signature": .string(PKCE.base64URL(assertion.signature)),
        ]
        if let user = assertion.userID, !user.isEmpty {
            response["userHandle"] = .string(PKCE.base64URL(user))
        }
        return .object([
            "id": .string(id),
            "rawId": .string(id),
            "type": .string("public-key"),
            "response": .object(response),
            "clientExtensionResults": .object([:]),
            "authenticatorAttachment": .string("platform"),
        ])
    }

    nonisolated func authorizationController(controller: ASAuthorizationController, didCompleteWithAuthorization authorization: ASAuthorization) {
        MainActor.assumeIsolated {
            if let assertion = authorization.credential as? ASAuthorizationPlatformPublicKeyCredentialAssertion {
                continuation?.resume(returning: assertion)
            } else {
                continuation?.resume(throwing: Failure.failed)
            }
            continuation = nil
        }
    }

    nonisolated func authorizationController(controller: ASAuthorizationController, didCompleteWithError error: Error) {
        MainActor.assumeIsolated {
            if let auth = error as? ASAuthorizationError, auth.code == .canceled {
                continuation?.resume(throwing: Failure.cancelled)
            } else {
                continuation?.resume(throwing: Failure.failed)
            }
            continuation = nil
        }
    }

    nonisolated func presentationAnchor(for controller: ASAuthorizationController) -> ASPresentationAnchor {
        MainActor.assumeIsolated { PresentationAnchor.window }
    }
}
