import 'package:flutter_probe_annotation/flutter_probe_annotation.dart';

@ProbeCompositeTest(
  name: 'alice sends bob a message',
  tags: ['composite', 'smoke'],
  devices: [
    Device('A', target: 'iPhone 15 Simulator'),
    Device('B', target: 'Pixel 9 Emulator'),
  ],
  body: [
    OnDevice('A', steps: [
      Open(),
      Tap(text: 'Sign in as Alice'),
      WaitUntil.appears('Inbox'),
    ]),
    OnDevice('B', steps: [
      Open(),
      Tap(text: 'Sign in as Bob'),
      WaitUntil.appears('Inbox'),
    ]),
    Sync('both signed in'),
    OnDevice('A', steps: [
      Tap(text: 'New message'),
      Type('hello bob'),
      Tap(text: 'Send'),
    ]),
    OnDevice('B', steps: [
      WaitUntil.appears('hello bob'),
      See('hello bob'),
    ]),
    Sync('message delivered'),
  ],
)
class ChatComposite {}
