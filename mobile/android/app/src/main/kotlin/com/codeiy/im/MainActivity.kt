package com.codeiy.im

import android.os.Bundle
import androidx.activity.ComponentActivity
import androidx.activity.compose.setContent
import androidx.activity.viewModels
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.material3.Button
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Surface
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.runtime.Composable
import androidx.compose.runtime.collectAsState
import androidx.compose.runtime.getValue
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.padding
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.unit.dp
import androidx.lifecycle.ViewModel
import androidx.lifecycle.ViewModelProvider
import androidx.navigation.NavType
import androidx.navigation.compose.NavHost
import androidx.navigation.compose.composable
import androidx.navigation.compose.rememberNavController
import androidx.navigation.navArgument
import androidx.navigation.navDeepLink
import com.codeiy.im.core.auth.SessionState
import com.codeiy.im.core.auth.SessionStore
import com.codeiy.im.core.auth.TokenStore
import com.codeiy.im.core.billing.RevenueCatBillingService
import com.codeiy.im.feature.inbox.InboxScreen
import com.codeiy.im.feature.login.LoginScreen
import com.codeiy.im.feature.opportunity.OpportunityDetailScreen

class MainActivity : ComponentActivity() {
    private val session: SessionStore by viewModels {
        object : ViewModelProvider.Factory {
            @Suppress("UNCHECKED_CAST")
            override fun <T : ViewModel> create(modelClass: Class<T>): T =
                SessionStore(TokenStore(applicationContext), RevenueCatBillingService(applicationContext)) as T
        }
    }

    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)
        setContent {
            MaterialTheme {
                Surface(modifier = Modifier.fillMaxSize()) {
                    RootView(session)
                }
            }
        }
    }
}

@Composable
private fun RootView(session: SessionStore) {
    val state by session.state.collectAsState()
    when (val current = state) {
        is SessionState.Restoring -> Box(Modifier.fillMaxSize(), contentAlignment = Alignment.Center) {
            CircularProgressIndicator()
        }
        is SessionState.RestoreFailed -> Column(
            modifier = Modifier.fillMaxSize().padding(24.dp),
            verticalArrangement = Arrangement.Center,
            horizontalAlignment = Alignment.CenterHorizontally,
        ) {
            Text("会话恢复失败", style = MaterialTheme.typography.titleMedium)
            Text(current.message, style = MaterialTheme.typography.bodyMedium)
            Button(onClick = { session.restore() }) { Text("重试") }
            TextButton(onClick = { session.logout() }) { Text("退出登录") }
        }
        is SessionState.LoggedOut -> LoginScreen(session)
        is SessionState.Active -> AppNavHost(session)
    }
}

@Composable
private fun AppNavHost(session: SessionStore) {
    val navController = rememberNavController()
    NavHost(navController, startDestination = "inbox") {
        composable("inbox") {
            InboxScreen(session) { id -> navController.navigate("opportunity/$id") }
        }
        composable(
            route = "opportunity/{opportunityId}",
            arguments = listOf(navArgument("opportunityId") { type = NavType.StringType }),
            // 推送/外链深链入口（对齐移动端计划「点击推送深链进详情」）。
            // https App Link 需在域名下发布 assetlinks.json 后才会直开 App，见 README。
            deepLinks = listOf(
                navDeepLink { uriPattern = "opportunity-radar://opportunity/{opportunityId}" },
                navDeepLink { uriPattern = "https://im.story2u.xyz/app/opportunity/{opportunityId}" },
            ),
        ) { entry ->
            val opportunityId = entry.arguments?.getString("opportunityId").orEmpty()
            OpportunityDetailScreen(
                session = session,
                opportunityId = opportunityId,
                onBack = { navController.popBackStack() },
            )
        }
    }
}
