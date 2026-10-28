// nt for iPhone and iPad (docs/design.md §9). Step 1: the core and git on the phone. The first
// run makes the phone's key and clones the notes; then the notes are listed and synced.
import SwiftUI

@main
struct NtApp: App {
    @State private var phone = Phone()

    var body: some Scene {
        WindowGroup {
            Group {
                if ProcessInfo.processInfo.environment["XCTestConfigurationFilePath"] != nil {
                    // the tests' host: nt in this process belongs to the tests
                    Color.clear
                } else if phone.cloned {
                    NotesView()
                } else {
                    SetupView()
                }
            }
            .environment(phone)
        }
    }
}
