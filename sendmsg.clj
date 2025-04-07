(require '[babashka.pods :as pods]
         '[clojure.string :as str]
         '[babashka.process :refer [process]])
(pods/load-pod "./bb-whatsapp-pod")

(require '[pod.whatsapp :as wa])


(let [login-result (wa/login)]
  (when (= (:status login-result) "qr-pending")
    (println "--- QR CODE ---")
    (-> (process ["qrencode" "-t" "ANSI256" "-o" "-" (:qr_code login-result)] {:out :inherit})
        deref)
    (println "---------------")
    (println "\nPlease scan the QR code string above using WhatsApp on your phone (Link a device).")
    (println "Press Enter here after you have scanned the QR code...")
    (read-line)
    (println "Checking status after scanning...")))
  

(let [status-result (wa/status)]
  (if (= (:status status-result) "logged-in")
    (do
      (println "\nPlease enter your WhatsApp phone number (country code without '+', e.g., 14155551234) to send a message:")
      (let [phone-number (str/trim (read-line))
            _ (println "Enter the message you want to send:")
            message (str/trim (read-line))]
        (if (re-matches #"^\d+$" phone-number)
          (do
            (println "Sending test message to" phone-number)
            (let [send-result (wa/send-message phone-number message)]
              (println "Send message result:" send-result)))
          (println "Invalid phone number format entered."))))))

(shutdown-agents) 