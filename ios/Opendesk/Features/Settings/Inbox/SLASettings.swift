import SwiftUI

/// Settings › SLA policies.
struct SLASettings: View {
    var body: some View {
        SettingsWebFallback(page: .sla)
    }

    struct Detail: View {
        let id: String

        var body: some View {
            SettingsWebFallback(page: .sla, id: id)
        }
    }
}
