package io.sild.sample

import android.os.Bundle
import androidx.activity.ComponentActivity
import androidx.activity.compose.setContent
import androidx.compose.foundation.background
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material3.Button
import androidx.compose.material3.Card
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedButton
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.rememberCoroutineScope
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import io.sild.core.SildConfig
import io.sild.ui.Sild
import io.sild.ui.SildMessenger
import kotlinx.coroutines.launch

// The sample "Acme Rides" app — the SDK's counterpart to web/demo.html. A trip
// card offers "Message driver" (peer chat via openConversation) and a support
// card offers "Open support" (openSupportRequest). Sild.init runs once; the host
// backend (DevBackend here) provides the token + the driver conversation id.
class MainActivity : ComponentActivity() {
    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)
        // With a Context, so published strings survive the app being killed.
        Sild.init(this, SildConfig(baseUrl = DevBackend.BASE, tokenProvider = DevBackend.tokenProvider, userId = DevBackend.USER_ID))
        setContent { AcmeRidesScreen() }
    }
}

@Composable
private fun AcmeRidesScreen() {
    val ctx = androidx.compose.ui.platform.LocalContext.current
    val scope = rememberCoroutineScope()
    val slate = Color(0xFF14181F)
    val brand = Color(0xFF2563FD)

    Column(Modifier.fillMaxSize().background(Color(0xFFF4F6FA))) {
        Row(
            Modifier.fillMaxWidth().background(slate).padding(16.dp),
            verticalAlignment = androidx.compose.ui.Alignment.CenterVertically,
        ) {
            Text("Acme Rides", color = Color.White, fontWeight = FontWeight.ExtraBold, fontSize = 20.sp, modifier = Modifier.weight(1f))
            OutlinedButton(onClick = { SildMessenger.openList(ctx) }) { Text("Messages") }
        }
        Column(Modifier.padding(16.dp), verticalArrangement = Arrangement.spacedBy(14.dp)) {
            // Trip-in-progress card → peer/driver chat (client.openConversation(tripId)).
            Card(Modifier.fillMaxWidth(), shape = RoundedCornerShape(12.dp)) {
                Column(Modifier.padding(16.dp)) {
                    Text("TRIP IN PROGRESS", color = Color(0xFF8A94A4), fontSize = 11.sp, fontWeight = FontWeight.SemiBold)
                    Text("Toomas · Silver estate · 421 KLM", color = slate, fontSize = 15.sp, fontWeight = FontWeight.Bold, modifier = Modifier.padding(top = 8.dp))
                    Text("Arriving in 2 min", color = Color(0xFF5B6472), fontSize = 13.sp, modifier = Modifier.padding(top = 2.dp))
                    OutlinedButton(
                        onClick = { scope.launch { SildMessenger.openConversation(ctx, DevBackend.ensureDriverConversation(DevBackend.DRIVER_TRIP_REF)) } },
                        modifier = Modifier.fillMaxWidth().padding(top = 14.dp),
                    ) { Text("Message driver") }
                }
            }
            // Support card → new support request (client.openSupportRequest()).
            Card(Modifier.fillMaxWidth(), shape = RoundedCornerShape(12.dp)) {
                Column(Modifier.padding(16.dp)) {
                    Text("Need a hand?", color = slate, fontSize = 15.sp, fontWeight = FontWeight.Bold)
                    Text("Questions about your pickup, payment, or your driver? An agent replies in a few minutes.", color = Color(0xFF5B6472), fontSize = 13.sp, modifier = Modifier.padding(top = 8.dp))
                    Button(
                        onClick = { SildMessenger.openSupportRequest(ctx) },
                        modifier = Modifier.fillMaxWidth().padding(top = 14.dp),
                        colors = androidx.compose.material3.ButtonDefaults.buttonColors(containerColor = brand),
                    ) { Text("Open support") }
                }
            }
            Text("client.openConversation(tripId) · client.openSupportRequest()", color = Color(0xFF8A94A4), fontSize = 11.sp, modifier = Modifier.padding(top = 4.dp))
        }
    }
}
