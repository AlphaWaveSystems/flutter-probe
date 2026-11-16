abstract class Widget {
  const Widget();
}

abstract class StatelessWidget extends Widget {
  const StatelessWidget();
}

abstract class StatefulWidget extends Widget {
  const StatefulWidget();
}

abstract class ProxyWidget extends Widget {
  const ProxyWidget();
}

abstract class InheritedWidget extends ProxyWidget {
  const InheritedWidget();
}

abstract class State<T extends StatefulWidget> {}

class ErrorWidget extends LeafRenderObjectWidget {}

abstract class LeafRenderObjectWidget extends Widget {}
