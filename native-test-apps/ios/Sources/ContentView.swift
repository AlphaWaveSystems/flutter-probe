import SwiftUI

/// Single-screen test target for FlutterProbe's iOS native UI bridging spike
/// (docs/proposals/n2-ios-native-ui-bridging.md).
///
/// Every element carries a stable, explicit accessibility identifier so it can
/// be matched by WebDriverAgent-style selectors (identifier / label).
struct ContentView: View {
    @State private var tapCount = 0
    @State private var inputText = ""
    @State private var switchOn = false
    @State private var messageVisible = false

    var body: some View {
        ScrollView {
            VStack(spacing: 20) {
                Text("Native Test App")
                    .font(.title)
                    .bold()
                    .accessibilityIdentifier("native_title")

                Button("Tap Me") {
                    tapCount += 1
                }
                .buttonStyle(.borderedProminent)
                .accessibilityIdentifier("native_button")

                Text("Taps: \(tapCount)")
                    .accessibilityIdentifier("native_counter")

                TextField("Enter text here", text: $inputText)
                    .textFieldStyle(.roundedBorder)
                    .autocorrectionDisabled()
                    .accessibilityIdentifier("native_input")

                Text("Echo: \(inputText.isEmpty ? "(empty)" : inputText)")
                    .accessibilityIdentifier("native_echo")

                Toggle("Native Switch", isOn: $switchOn)
                    .accessibilityIdentifier("native_switch")

                Button("Show Message") {
                    messageVisible = true
                }
                .buttonStyle(.bordered)
                .accessibilityIdentifier("native_message_button")

                if messageVisible {
                    Text("Message Revealed")
                        .foregroundStyle(.green)
                        .accessibilityIdentifier("native_message")
                }

                Spacer()
            }
            .padding(24)
        }
    }
}

#Preview {
    ContentView()
}
