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

      ;; List groups
      (println "\nFetching your groups...")
      (let [groups-result (wa/get-groups)]
        (if (:success groups-result)
          (do
            (println "\nYour groups:")
            (doseq [group (:groups groups-result)]
              (println "\nGroup:" (:name group))
              (println "JID:" (:jid group))
              (println "Participants:" (:participants group)))

            ;; Example of sending a message to a group
            (println "\nWould you like to send a message to a group? (y/n)")
            (when (= (str/trim (read-line)) "y")
              (println "\nEnter the group JID (e.g., 1234567890@g.us):")
              (let [group-jid (str/trim (read-line))
                    _ (println "Enter the message you want to send:")
                    message (str/trim (read-line))]
                (println "Sending message to group" group-jid)
                (let [send-result (wa/send-group-message group-jid message)]
                  (println "Send message result:" send-result)))))
          (println "Failed to fetch groups:" (:message groups-result))))

      ;; Example of sending a message to a contact
      (println "\nWould you like to send a message to a contact? (y/n)")
      (when (= (str/trim (read-line)) "y")
        (println "\nPlease enter your WhatsApp phone number (country code without '+', e.g., 14155551234) to send a test message:")
        (let [phone-number (str/trim (read-line))]
          (if (re-matches #"^\d+$" phone-number)
            (do
              (println "Sending test message to" phone-number)
              (let [send-result (wa/send-message phone-number (str "Hello from Babashka pod! Timestamp: " (java.time.Instant/now)))]
                (println "Send message result:" send-result)))
            (println "Invalid phone number format entered.")))))

    (println "\nLogin was not successful. Skipping message sending and logout.")))

(println "\nExample script finished.")
(shutdown-agents) 