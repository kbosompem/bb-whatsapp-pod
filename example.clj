(require '[babashka.pods :as pods]
         '[clojure.string :as str]
         '[babashka.process :refer [process]])
(pods/load-pod "./bb-whatsapp-pod")

(require '[pod.whatsapp :as wa])


(let [login-result (wa/login)]
  (if (= (:status login-result) "qr-pending")
    (do
      (println "--- QR CODE ---")
      (-> (process ["qrencode" "-t" "ANSI256" "-o" "-" (:qr_code login-result)] {:out :inherit})
          deref)
      (println "---------------")
      (println "\nPlease scan the QR code string above using WhatsApp on your phone (Link a device).")
      (println "Press Enter here after you have scanned the QR code...")
      (read-line)
      (println "Checking status after scanning..."))
    (if (= (:status login-result) "logged-in")
      (println "Already logged in.")
      (println "Login status wasn't 'qr-pending' or 'logged-in', proceeding..."))))


(let [status-result (wa/status)]
  (if (= (:status status-result) "logged-in")
    (do
      (println "\nSuccessfully logged in!")
      (println "\nPlease enter your WhatsApp phone number (country code without '+', e.g., 14155551234) to send a test message:")
      (let [phone-number (str/trim (read-line))]
        (if (re-matches #"^\d+$" phone-number)
          (do
            (println "Sending test message to" phone-number)
            (let [send-result (wa/send-message phone-number (str "Hello from Babashka pod! Timestamp: " (java.time.Instant/now)))]
              (println "Send message result:" send-result)))
          (println "Invalid phone number format entered.")))

      (println "\nLogging out...")
      (let [logout-result (wa/logout)]
        (println "Logout result:" logout-result))

      (println "\nChecking status after logout...")
      (let [final-status (wa/status)]
        (println "Final status:" final-status)))

    (println "\nLogin was not successful. Skipping message sending and logout.")))

(println "\nExample script finished.")



(shutdown-agents) 