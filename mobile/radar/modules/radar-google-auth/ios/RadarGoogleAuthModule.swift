import ExpoModulesCore
import GoogleSignIn
import UIKit

private let googleSignInCancellationErrorCode = -5

private final class GoogleIdentityUnavailableException: Exception, @unchecked Sendable {
  override var reason: String {
    "Google sign-in is unavailable"
  }
}

private final class GoogleIdentityTokenMissingException: Exception, @unchecked Sendable {
  override var reason: String {
    "Google sign-in did not return an identity token"
  }
}

public final class RadarGoogleAuthModule: Module {
  public func definition() -> ModuleDefinition {
    Name("RadarGoogleAuth")

    AsyncFunction("signInAsync") { (serverClientId: String, iosClientId: String?, promise: Promise) in
      guard let iosClientId, !iosClientId.isEmpty else {
        promise.reject(GoogleIdentityUnavailableException())
        return
      }

      DispatchQueue.main.async {
        guard let viewController = self.appContext?.utilities?.currentViewController() else {
          promise.reject(GoogleIdentityUnavailableException())
          return
        }

        GIDSignIn.sharedInstance.configuration = GIDConfiguration(
          clientID: iosClientId,
          serverClientID: serverClientId
        )
        GIDSignIn.sharedInstance.signIn(withPresenting: viewController) { result, error in
          if let error {
            let signInError = error as NSError
            if signInError.domain == kGIDSignInErrorDomain
              && signInError.code == googleSignInCancellationErrorCode {
              promise.resolve(["type": "cancelled"])
            } else {
              promise.reject(GoogleIdentityUnavailableException())
            }
            return
          }
          guard let idToken = result?.user.idToken?.tokenString else {
            promise.reject(GoogleIdentityTokenMissingException())
            return
          }
          promise.resolve(["type": "success", "idToken": idToken])
        }
      }
    }
  }
}

public final class RadarGoogleAuthAppDelegateSubscriber: ExpoAppDelegateSubscriber {
  public func application(
    _ app: UIApplication,
    open url: URL,
    options: [UIApplication.OpenURLOptionsKey: Any] = [:]
  ) -> Bool {
    GIDSignIn.sharedInstance.handle(url)
  }
}
