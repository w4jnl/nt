// nt's settings as nt config --json lists them: this Mac's (config.toml) and every machine's
// (.nt/config.toml in the repository), each with its kind, its value and what it does. Settings
// shows them and changes one through nt config set, which keeps the file's comments and checks
// the result loads (docs/design.md §4, Settings).
import Foundation

struct ConfigSetting: Decodable, Equatable, Identifiable, Sendable {
    let key: String
    let kind: String // bool, enum, string, list, time (HH:MM), duration
    let choices: [String]?
    let value: String
    let help: String
    let file: String // machine or repo
    let path: String

    var id: String { file + ":" + key }

    struct List: Decodable, Sendable {
        let settings: [ConfigSetting]
    }

    /// applies says what the app does once the setting changed: read the repository again
    /// (its folders, pins and day), start over on another repository, or nothing.
    enum Effect: Equatable { case none, reindex, restart }

    nonisolated static func effect(of key: String) -> Effect {
        switch key {
        case "repo": .restart
        case "pins", "day_starts", "default_folder", "folders.daily", "folders.agents", "folders.weekly": .reindex
        default: .none
        }
    }
}

/// TextSize is how large the note's text is, the editor page's zoom; kept on this Mac.
enum TextSize: Double, CaseIterable, Identifiable, Sendable {
    case small = 0.9
    case standard = 1.0
    case large = 1.15
    case larger = 1.3

    var id: Double { rawValue }

    var name: String {
        switch self {
        case .small: "Small"
        case .standard: "Standard"
        case .large: "Large"
        case .larger: "Larger"
        }
    }
}
