package dev.probe.webviewfixture;

import android.app.Activity;
import android.os.Bundle;
import android.webkit.WebView;

import java.io.ByteArrayOutputStream;
import java.io.IOException;
import java.io.InputStream;

/** One WebView that shows assets/page.html, with WebView debugging on. It is the target of the WebView steps (see README.md). */
public class MainActivity extends Activity {
    @Override
    protected void onCreate(Bundle state) {
        super.onCreate(state);
        WebView.setWebContentsDebuggingEnabled(true);
        WebView web = new WebView(this);
        web.getSettings().setJavaScriptEnabled(true);
        web.loadDataWithBaseURL("https://fixture.invalid/", readPage(), "text/html", "utf-8", null);
        setContentView(web);
    }

    private String readPage() {
        try (InputStream in = getAssets().open("page.html")) {
            ByteArrayOutputStream out = new ByteArrayOutputStream();
            byte[] buf = new byte[4096];
            for (int n; (n = in.read(buf)) > 0; ) {
                out.write(buf, 0, n);
            }
            return out.toString("UTF-8");
        } catch (IOException e) {
            return "<html><body>page.html could not be read</body></html>";
        }
    }
}
