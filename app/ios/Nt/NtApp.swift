// nt for iPhone and iPad (docs/design.md §9). The first run makes the phone's key and clones the
// notes; then the home list, the notes in live preview and the views, as on the Mac: one stack on
// an iPhone, the sidebar beside the note on an iPad.
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
                } else {
                    RootView()
                }
            }
            .environment(phone)
        }
    }
}
