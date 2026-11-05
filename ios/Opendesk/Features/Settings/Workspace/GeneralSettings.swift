import SwiftUI

/// Settings › General.
struct GeneralSettings: View {
    var body: some View {
        SettingsWebFallback(page: .general)
    }

    struct Detail: View {
        let id: String

        var body: some View {
            SettingsWebFallback(page: .general, id: id)
        }
    }
}
