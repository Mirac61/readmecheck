workspace "readmecheck" "Architecture of the readmecheck README auditor" {

    model {
        user = person "Developer" "Runs readmecheck locally and reviews the report."

        readmecheck = softwareSystem "readmecheck" "Local-first Go web application that audits GitHub READMEs against 23 deterministic rules." {
            app = container "readmecheck server" "HTTP server, rule engine, and server-rendered templ UI." "Go" {
                web = component "Web layer" "Routes, handlers, and templ/htmx views." "internal/web"
                githubClient = component "GitHub client" "Fetches repository data and the README." "internal/github"
                rules = component "Rule engine" "Runs 23 checks and scores the README." "internal/rules"
                markdown = component "Markdown parser" "Builds the document model with goldmark." "internal/markdown"
                store = component "History store" "Persists checks and serves recent results." "internal/store"
            }

            database = container "History database" "One row per check." "SQLite" {
                tags "Database"
            }
        }

        github = softwareSystem "GitHub REST API" "Repository metadata, README, workflows, releases, and file tree." {
            tags "External System"
        }

        externalLinks = softwareSystem "External websites" "Links found in the README." {
            tags "External System"
        }

        user -> web "Submits a repository URL"
        web -> githubClient "Fetches a repository snapshot"
        githubClient -> github "Requests repository data" "HTTPS / JSON"
        web -> rules "Runs the checklist"
        rules -> markdown "Parses the README"
        rules -> externalLinks "Checks links (optional)" "HTTPS"
        web -> store "Saves checks and loads history"
        store -> database "Reads and writes checks" "SQL"
    }

    views {
        systemContext readmecheck "SystemContext" {
            include *
            autoLayout lr
        }

        container readmecheck "Containers" {
            include *
            autoLayout lr
        }

        component app "Components" {
            include *
            autoLayout lr
        }

        styles {
            element "Person" {
                shape Person
            }
            element "Database" {
                shape Cylinder
            }
            element "External System" {
                background "#999999"
                color "#ffffff"
            }
        }
    }
}
