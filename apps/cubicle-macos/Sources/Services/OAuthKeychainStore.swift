import Foundation
import LocalAuthentication
import Security

/// Keychain access wrapper for OAuth and transcription secrets.
struct OAuthKeychainStore {
    private let serviceName = "com.cubicle.oauth"

    /// Loads an OAuth access token by provider.
    func loadAccessToken(provider: OAuthProviderKind, allowUserInteraction: Bool = false) -> String? {
        loadSecret(account: accountName(for: provider), allowUserInteraction: allowUserInteraction)
    }

    /// Loads a raw secret by Keychain account name.
    func loadSecret(account: String, allowUserInteraction: Bool = true) -> String? {
        var query: [String: Any] = [
            kSecClass as String: kSecClassGenericPassword,
            kSecAttrService as String: serviceName,
            kSecAttrAccount as String: account,
            kSecReturnData as String: true,
            kSecMatchLimit as String: kSecMatchLimitOne
        ]
        if !allowUserInteraction {
            let context = LAContext()
            context.interactionNotAllowed = true
            query[kSecUseAuthenticationContext as String] = context
            query[kSecUseAuthenticationUI as String] = kSecUseAuthenticationUISkip
        }

        var result: CFTypeRef?
        let status = SecItemCopyMatching(query as CFDictionary, &result)
        guard status == errSecSuccess,
              let data = result as? Data,
              let value = String(data: data, encoding: .utf8)?
                .trimmingCharacters(in: .whitespacesAndNewlines),
              !value.isEmpty else {
            return nil
        }
        return value
    }

    /// Checks for a secret without returning its value.
    func secretExists(account: String) -> Bool {
        let context = LAContext()
        context.interactionNotAllowed = true
        let query: [String: Any] = [
            kSecClass as String: kSecClassGenericPassword,
            kSecAttrService as String: serviceName,
            kSecAttrAccount as String: account,
            kSecReturnData as String: false,
            kSecMatchLimit as String: kSecMatchLimitOne,
            kSecUseAuthenticationContext as String: context,
            kSecUseAuthenticationUI as String: kSecUseAuthenticationUISkip
        ]
        return SecItemCopyMatching(query as CFDictionary, nil) == errSecSuccess
    }

    /// Saves an OAuth access token under the provider account.
    func saveAccessToken(_ token: String, provider: OAuthProviderKind) throws {
        try saveSecret(token, account: accountName(for: provider), description: "OAuth token")
    }

    /// Saves a raw secret under a Keychain account name.
    func saveSecret(_ token: String, account: String, description: String = "secret") throws {
        let normalized = token.trimmingCharacters(in: .whitespacesAndNewlines)
        guard !normalized.isEmpty else {
            return
        }
        let payload = Data(normalized.utf8)
        let query: [String: Any] = [
            kSecClass as String: kSecClassGenericPassword,
            kSecAttrService as String: serviceName,
            kSecAttrAccount as String: account
        ]

        let attributes: [String: Any] = [
            kSecValueData as String: payload
        ]
        let updateStatus = SecItemUpdate(query as CFDictionary, attributes as CFDictionary)
        if updateStatus == errSecSuccess {
            return
        }
        if updateStatus != errSecItemNotFound {
            throw NSError(
                domain: NSOSStatusErrorDomain,
                code: Int(updateStatus),
                userInfo: [NSLocalizedDescriptionKey: "Could not update \(description) in keychain (status \(updateStatus))."]
            )
        }

        var create = query
        create[kSecValueData as String] = payload
        create[kSecAttrAccessible as String] = kSecAttrAccessibleAfterFirstUnlock
        let createStatus = SecItemAdd(create as CFDictionary, nil)
        guard createStatus == errSecSuccess else {
            throw NSError(
                domain: NSOSStatusErrorDomain,
                code: Int(createStatus),
                userInfo: [NSLocalizedDescriptionKey: "Could not save \(description) to keychain (status \(createStatus))."]
            )
        }
    }

    /// Deletes an OAuth access token by provider.
    func deleteAccessToken(provider: OAuthProviderKind) throws {
        try deleteSecret(account: accountName(for: provider), description: "OAuth token")
    }

    /// Deletes a raw secret by Keychain account name.
    func deleteSecret(account: String, description: String = "secret") throws {
        let query: [String: Any] = [
            kSecClass as String: kSecClassGenericPassword,
            kSecAttrService as String: serviceName,
            kSecAttrAccount as String: account
        ]
        let status = SecItemDelete(query as CFDictionary)
        guard status == errSecSuccess || status == errSecItemNotFound else {
            throw NSError(
                domain: NSOSStatusErrorDomain,
                code: Int(status),
                userInfo: [NSLocalizedDescriptionKey: "Could not delete \(description) from keychain (status \(status))."]
            )
        }
    }

    private func accountName(for provider: OAuthProviderKind) -> String {
        "\(provider.rawValue).access_token"
    }
}
