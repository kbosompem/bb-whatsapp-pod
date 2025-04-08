(require '[babashka.pods :as pods]
         '[clojure.pprint :as pprint]
         '[clojure.string :as str])

(pods/load-pod "./bb-whatsapp-pod")
(require '[pod.whatsapp :as wa])

(defn prepare-groups-data [groups]
  (map (fn [group]
         {:name (:name group)
          :jid (str/replace (:jid group) #"@g\.us$" "")  ; Remove @g.us suffix for cleaner display
          :participants (count (:participants group))})
       (sort-by :name groups)))

(println "\nFetching WhatsApp groups...")

(let [login-result (wa/login) 
      status-result (wa/status)]
  (if (= (:status status-result) "logged-in")
    (let [groups-result (wa/get-groups)]
      (if (:success groups-result)
        (do
          (println "\nWhatsApp Groups Summary:")
          (pprint/print-table (prepare-groups-data (:groups groups-result)))
          (println "\nTotal groups:" (count (:groups groups-result))))
        (println "Failed to fetch groups:" (:message groups-result))))
    (println "Not logged in to WhatsApp. Please run sendmsg.clj first to log in.")))

(shutdown-agents) 