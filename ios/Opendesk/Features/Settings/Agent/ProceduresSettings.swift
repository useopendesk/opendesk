import SwiftUI

/// Settings › AI agent › Procedures.
struct ProceduresSettings: View {
    var body: some View {
        SettingsWebFallback(page: .procedures)
    }

    struct Detail: View {
        let id: String

        var body: some View {
            SettingsWebFallback(page: .procedures, id: id)
        }
    }
}
