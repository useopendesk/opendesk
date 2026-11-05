import SwiftUI

/// Settings › Inbox categories.
struct MailCategoriesSettings: View {
    var body: some View {
        SettingsWebFallback(page: .mailCategories)
    }

    struct Detail: View {
        let id: String

        var body: some View {
            SettingsWebFallback(page: .mailCategories, id: id)
        }
    }
}
