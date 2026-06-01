import Foundation
import LocalAuthentication
import Security

struct OAuthKeychainStore {
    private let serviceName = "com.cubicle.oauth"

    func loadAccessToken(provider: OAuthProviderKind, allowUserInteraction: Bool = false) -> String? {
        loadSecret(account: accountName(for: provider), allowUserInteraction: allowUserInteraction)
    }

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

    func saveAccessToken(_ token: String, provider: OAuthProviderKind) throws {
        try saveSecret(token, account: accountName(for: provider), description: "OAuth token")
    }

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

    func deleteAccessToken(provider: OAuthProviderKind) throws {
        try deleteSecret(account: accountName(for: provider), description: "OAuth token")
    }

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
