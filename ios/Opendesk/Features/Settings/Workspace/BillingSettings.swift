import SwiftUI

/// Settings › Plan and billing.
struct BillingSettings: View {
    var body: some View {
        SettingsWebFallback(page: .billing)
    }

    struct Detail: View {
        let id: String

        var body: some View {
            SettingsWebFallback(page: .billing, id: id)
        }
    }
}
