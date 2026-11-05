import SwiftUI

/// Settings › Triage.
struct TriageSettings: View {
    var body: some View {
        SettingsWebFallback(page: .triage)
    }

    struct Detail: View {
        let id: String

        var body: some View {
            SettingsWebFallback(page: .triage, id: id)
        }
    }
}
