import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:flutter_probe_agent/src/executor.dart';

void main() {
  test('no distance is the default, 50 percent', () {
    expect(swipeFraction(0), 0.5);
  });

  test('a distance is that percent of the element or the screen', () {
    expect(swipeFraction(40), 0.4);
    expect(swipeFraction(1), 0.01);
    expect(swipeFraction(100), 1.0);
  });

  test(
      'a distance above 100 is clamped to 100, and a negative one is the default',
      () {
    expect(swipeFraction(150), 1.0);
    expect(swipeFraction(-5), 0.5);
  });

  test(
      'the delta is the size times the fraction, in the direction of the swipe',
      () {
    const size = Size(400, 800);
    expect(swipeDelta('up', size, 0.5), const Offset(0, -400));
    expect(swipeDelta('down', size, 0.5), const Offset(0, 400));
    expect(swipeDelta('left', size, 0.5), const Offset(-200, 0));
    expect(swipeDelta('right', size, 0.5), const Offset(200, 0));
  });

  test(
      'a distance is applied once: 40 percent of the height is 320, not 40 percent of 40 percent',
      () {
    const size = Size(400, 800);
    expect(swipeDelta('up', size, swipeFraction(40)), const Offset(0, -320));
  });

  test(
      '100 percent ends one half-size past the edge: from the centre at 400, the finger ends at -400',
      () {
    const size = Size(400, 800);
    expect(swipeDelta('up', size, swipeFraction(100)), const Offset(0, -800));
  });

  // The target at (100, 200), 400 wide and 800 high: its centre is (300, 600).
  const origin = Offset(100, 200);
  const box = Size(400, 800);

  test(
      'a swipe with a start begins that percent in from the edge it starts at, on the centre of the other axis',
      () {
    expect(swipeStart('up', origin, box, 10),
        const Offset(300, 920)); // 10 percent above the bottom edge
    expect(swipeStart('down', origin, box, 10),
        const Offset(300, 280)); // 10 percent below the top edge
    expect(swipeStart('left', origin, box, 10),
        const Offset(460, 600)); // 10 percent in from the right edge
    expect(swipeStart('right', origin, box, 10),
        const Offset(140, 600)); // 10 percent in from the left edge
  });

  test(
      '100 percent starts on the far edge of the target, and the clamp keeps anything above it there',
      () {
    expect(swipeStart('up', origin, box, 100),
        const Offset(300, 200)); // the top edge, for a swipe up from the bottom
    expect(swipeStart('right', origin, box, 100),
        const Offset(500, 600)); // the right edge
    expect(swipeStart('right', origin, box, 150), const Offset(500, 600));
  });

  test('50 percent is the centre on the swipe axis, the same point as no start',
      () {
    expect(swipeStart('up', origin, box, 50), const Offset(300, 600));
    expect(swipeStart('left', origin, box, 50), const Offset(300, 600));
  });

  test('an unknown direction is a right swipe, as swipeDelta treats it', () {
    expect(swipeStart('sideways', origin, box, 2),
        swipeStart('right', origin, box, 2));
  });
}
