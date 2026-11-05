import SwiftUI

/// Settings › Custom fields.
struct CustomFieldsSettings: View {
    var body: some View {
        SettingsWebFallback(page: .customFields)
    }

    struct Detail: View {
        let id: String

        var body: some View {
            SettingsWebFallback(page: .customFields, id: id)
        }
    }
}
