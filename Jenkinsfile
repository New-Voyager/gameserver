pipeline {
    agent any
    options {
        disableConcurrentBuilds()
    }
    stages {
        stage('env') {
            steps {
                sh 'printenv | sort'
                sh 'pwd'
                sh 'ls -l'
            }
        }
        stage('Clean Up Containers') {
            steps {
                cleanUpContainers()
            }
        }
        stage('Build') {
            steps {
                sh 'make docker-build'
            }
        }
        stage('Unit Test') {
            steps {
                sh 'make test'
            }
        }
        stage('Docker Test') {
            steps {
                sh 'mkdir -p jenkins_logs'
                echo 'Running Docker Test. Will save the log as artifact (docker_test.log).'
                sh 'make docker-test > jenkins_logs/docker_test.log 2>&1'
            }
        }
        stage('System Test') {
            steps {
                echo 'Running System Test. Will save the log as artifact (system_test.log).'
                sh 'make system-test > jenkins_logs/system_test.log 2>&1'
            }
        }
        stage('Publish') {
            when {
                expression { return env.GIT_BRANCH == 'master' }
            }
            steps {
                sh 'make publish'
            }
        }
    }
    post {
        always {
            script {
                if (fileExists('jenkins_logs/docker_test.log')) {
                    echo '''###################################################
                            |Last 200 lines of docker test log
                            |###################################################
                         '''.stripMargin().stripIndent()
                    sh 'tail -n 200 jenkins_logs/docker_test.log'
                }
                if (fileExists('jenkins_logs/system_test.log')) {
                    echo '''###################################################
                            |Last 200 lines of system test log
                            |###################################################
                         '''.stripMargin().stripIndent()
                    sh 'tail -n 200 jenkins_logs/system_test.log'
                }
            }
            archiveArtifacts artifacts: 'jenkins_logs/*', allowEmptyArchive: true
            cleanUpContainers()
        }
        success {
            setBuildStatus("Build succeeded", "SUCCESS");
        }
        failure {
            setBuildStatus("Build failed", "FAILURE");
        }
    }
}

def setBuildStatus(String message, String state) {
    repoUrl = getRepoURL()
    step([
        $class: "GitHubCommitStatusSetter",
        reposSource: [$class: "ManuallyEnteredRepositorySource", url: repoUrl],
        contextSource: [$class: "ManuallyEnteredCommitContextSource", context: "ci/jenkins/build-status"],
        errorHandlers: [[$class: "ChangingBuildStatusErrorHandler", result: "UNSTABLE"]],
        statusResultSource: [ $class: "ConditionalStatusResultSource", results: [[$class: "AnyBuildResult", message: message, state: state]] ]
    ]);
}

def getRepoURL() {
  sh "git config --get remote.origin.url > .git/remote-url"
  return readFile(".git/remote-url").trim()
}

def cleanUpContainers() {
    sh 'docker rm -f $(docker ps | grep -v "jenkins" | awk \'{print $1}\' | tail -n +2) || true'
}
