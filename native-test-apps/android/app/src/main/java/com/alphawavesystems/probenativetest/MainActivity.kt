package com.alphawavesystems.probenativetest

import android.app.Activity
import android.os.Bundle
import android.text.Editable
import android.text.TextWatcher
import android.view.View
import android.widget.Button
import android.widget.EditText
import android.widget.TextView

class MainActivity : Activity() {

    private var tapCount = 0

    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)
        setContentView(R.layout.activity_main)

        val counter = findViewById<TextView>(R.id.native_counter)
        findViewById<Button>(R.id.native_button).setOnClickListener {
            tapCount++
            counter.text = "Taps: $tapCount"
        }

        val echo = findViewById<TextView>(R.id.native_echo)
        findViewById<EditText>(R.id.native_input).addTextChangedListener(object : TextWatcher {
            override fun beforeTextChanged(s: CharSequence?, start: Int, count: Int, after: Int) {}
            override fun onTextChanged(s: CharSequence?, start: Int, before: Int, count: Int) {}
            override fun afterTextChanged(s: Editable?) {
                val text = s?.toString().orEmpty()
                echo.text = if (text.isEmpty()) "Echo: (empty)" else "Echo: $text"
            }
        })

        val message = findViewById<TextView>(R.id.native_message)
        findViewById<Button>(R.id.native_message_button).setOnClickListener {
            message.visibility = View.VISIBLE
        }
    }
}
