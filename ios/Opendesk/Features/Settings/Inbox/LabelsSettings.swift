import SwiftUI

/// Settings › Labels.
struct LabelsSettings: View {
    var body: some View {
        SettingsWebFallback(page: .labels)
    }

    struct Detail: View {
        let id: String

        var body: some View {
            SettingsWebFallback(page: .labels, id: id)
        }
    }
}
