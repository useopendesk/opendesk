import SwiftUI

/// Settings › Brands and widgets.
struct BrandsSettings: View {
    var body: some View {
        SettingsWebFallback(page: .brands)
    }

    struct Detail: View {
        let id: String

        var body: some View {
            SettingsWebFallback(page: .brands, id: id)
        }
    }
}
