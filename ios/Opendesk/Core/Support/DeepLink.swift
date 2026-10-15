import Foundation

/// Where a link, a notification or a push takes the app.
///
/// Ticket URLs follow the web routes: `/inbox/<folder>/<ticket_id>` and
/// `/views/<view_id>/<ticket_id>` (the last segment is the ticket id). They
/// arrive as universal links on the instance's app origin, or as
/// `opendesk://open?path=/inbox/mine/<ticket_id>`.
enum DeepLink: Hashable, Sendable {
    case ticket(id: String, scope: InboxScope?)
    case scope(InboxScope)
    case notifications

    /// From a push or a link that names its server and workspace.
    struct Target: Hashable, Sendable {
        var link: DeepLink
        var server: String?
        var workspaceID: String?
    }

    /// An app path such as `/inbox/mine/<id>`, `/views/<view>/<id>` or `/inbox/open`.
    static func parse(path raw: String) -> DeepLink? {
        let path = raw.split(separator: "?", maxSplits: 1).first.map(String.init) ?? raw
        let segments = path.split(separator: "/").map { String($0).removingPercentEncoding ?? String($0) }.filter { !$0.isEmpty }
        guard let head = segments.first else { return nil }
        switch head {
        case "inbox":
            let folder = segments.count > 1 ? Folder(rawValue: segments[1]) : nil
            if segments.count >= 3, let id = segments.last {
                return .ticket(id: id, scope: folder.map(InboxScope.folder))
            }
            return .scope(.folder(folder ?? .mine))
        case "views":
            guard segments.count >= 2 else { return nil }
            let view = InboxScope.view(segments[1])
            if segments.count >= 3, let id = segments.last {
                return .ticket(id: id, scope: view)
            }
            return .scope(view)
        case "notifications":
            return .notifications
        default:
            return nil
        }
    }

    /// A universal link on `instance`'s app origin, or an `opendesk://open?path=` link.
    static func parse(url: URL, instance: Instance) -> Target? {
        if url.scheme?.lowercased() == "opendesk" {
            guard url.host()?.lowercased() == "open",
                  let components = URLComponents(url: url, resolvingAgainstBaseURL: false),
                  let path = components.queryItems?.first(where: { $0.name == "path" })?.value,
                  let link = parse(path: path)
            else { return nil }
            let server = components.queryItems?.first(where: { $0.name == "server" })?.value
            let workspace = components.queryItems?.first(where: { $0.name == "workspace" })?.value
            return Target(link: link, server: server, workspaceID: workspace)
        }
        guard let scheme = url.scheme?.lowercased(), scheme == "https" || scheme == "http",
              let host = url.host()?.lowercased()
        else { return nil }
        // Links for another server are opened on the web instead.
        guard host == instance.appOrigin.host()?.lowercased(), let link = parse(path: url.path()) else { return nil }
        return Target(link: link, server: instance.apiURL.absoluteString, workspaceID: nil)
    }
}
