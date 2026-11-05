import SwiftUI

/// Settings › Audit log.
struct AuditLogSettings: View {
    var body: some View {
        SettingsWebFallback(page: .auditLog)
    }

    struct Detail: View {
        let id: String

        var body: some View {
            SettingsWebFallback(page: .auditLog, id: id)
        }
    }
}
