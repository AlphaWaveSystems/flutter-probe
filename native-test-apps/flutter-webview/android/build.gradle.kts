allprojects {
    repositories {
        google()
        mavenCentral()
    }
}

val newBuildDir: Directory =
    rootProject.layout.buildDirectory
        .dir("../../build")
        .get()
rootProject.layout.buildDirectory.value(newBuildDir)

subprojects {
    val newSubprojectBuildDir: Directory = newBuildDir.dir(project.name)
    project.layout.buildDirectory.value(newSubprojectBuildDir)
}
// The agent's bonsoir_android plugin declares compileSdk 33 while its own
// androidx dependencies require 34+ — force every plugin subproject up to
// the app's compileSdk so AAR metadata checks pass. Must be registered
// before evaluationDependsOn(":app") below, which eagerly evaluates :app's
// project dependencies (including this plugin) as part of forcing :app's
// own evaluation order — afterEvaluate can't attach to an already-evaluated
// project.
subprojects {
    afterEvaluate {
        extensions.findByName("android")?.let { ext ->
            (ext as com.android.build.gradle.BaseExtension).compileSdkVersion(35)
        }
    }
}

subprojects {
    project.evaluationDependsOn(":app")
}

tasks.register<Delete>("clean") {
    delete(rootProject.layout.buildDirectory)
}
