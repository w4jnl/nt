// nt for iPhone and iPad (docs/design.md §9). The first run makes the phone's key and clones the
// notes; then the home list, the notes in live preview and the views, as on the Mac: one stack on
// an iPhone, the sidebar beside the note on an iPad. Shortcuts and Siri add to it without it on
// screen (Shortcuts.swift).
import AppIntents
import BackgroundTasks
import SwiftUI

@main
struct NtApp: App {
    @State private var phone: Phone
    @Environment(\.scenePhase) private var phase
    @State private var away = false

    init() {
        let phone = Phone()
        _phone = State(initialValue: phone)
        // the Shortcuts write through this phone's nt, also with the app off screen
        AppDependencyManager.shared.add(dependency: phone)
        BGTaskScheduler.shared.register(forTaskWithIdentifier: Phone.refreshID, using: .main) { task in
            guard let task = task as? BGAppRefreshTask else { return }
            MainActor.assumeIsolated { phone.refresh(task) }
        }
    }

    var body: some Scene {
        WindowGroup {
            Group {
                if ProcessInfo.processInfo.environment["XCTestConfigurationFilePath"] != nil {
                    // the tests' host: nt in this process belongs to the tests
                    Color.clear
                } else {
                    RootView()
                }
            }
            .environment(phone)
        }
        .onChange(of: phase) {
            // sync when the person leaves the app, and when they come back to it
            switch phase {
            case .background:
                away = true
                phone.leave()
            case .active where away:
                away = false
                phone.back()
            default:
                break
            }
        }
    }
}
