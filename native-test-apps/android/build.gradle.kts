// Root build file — plugin versions are declared in settings.gradle.kts.
tasks.register<Delete>("clean") {
    delete(rootProject.layout.buildDirectory)
}
