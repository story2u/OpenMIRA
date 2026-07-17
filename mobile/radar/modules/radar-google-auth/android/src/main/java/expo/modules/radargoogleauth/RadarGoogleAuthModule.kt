package expo.modules.radargoogleauth

import androidx.credentials.CredentialManager
import androidx.credentials.CustomCredential
import androidx.credentials.GetCredentialRequest
import androidx.credentials.exceptions.GetCredentialCancellationException
import androidx.credentials.exceptions.GetCredentialException
import com.google.android.libraries.identity.googleid.GetSignInWithGoogleOption
import com.google.android.libraries.identity.googleid.GoogleIdTokenCredential
import com.google.android.libraries.identity.googleid.GoogleIdTokenParsingException
import expo.modules.kotlin.exception.CodedException
import expo.modules.kotlin.functions.Coroutine
import expo.modules.kotlin.modules.Module
import expo.modules.kotlin.modules.ModuleDefinition

internal class GoogleIdentityException(cause: Throwable? = null) :
  CodedException("Google sign-in could not produce a valid identity token", cause)

class RadarGoogleAuthModule : Module() {
  override fun definition() = ModuleDefinition {
    Name("RadarGoogleAuth")

    AsyncFunction("signInAsync") Coroutine { serverClientId: String, _: String? ->
      val activity = appContext.throwingActivity
      val option = GetSignInWithGoogleOption.Builder(serverClientId).build()
      val request = GetCredentialRequest.Builder()
        .addCredentialOption(option)
        .build()

      try {
        val credential = CredentialManager.create(activity)
          .getCredential(context = activity, request = request)
          .credential
        if (
          credential !is CustomCredential ||
          credential.type != GoogleIdTokenCredential.TYPE_GOOGLE_ID_TOKEN_CREDENTIAL
        ) {
          throw GoogleIdentityException()
        }
        val idToken = GoogleIdTokenCredential.createFrom(credential.data).idToken
        mapOf("type" to "success", "idToken" to idToken)
      } catch (_: GetCredentialCancellationException) {
        mapOf("type" to "cancelled")
      } catch (error: GoogleIdTokenParsingException) {
        throw GoogleIdentityException(error)
      } catch (error: GetCredentialException) {
        throw GoogleIdentityException(error)
      }
    }
  }
}
