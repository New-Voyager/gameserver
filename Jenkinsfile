pipeline {
    agent any
    stages {
        stage('env') {
            steps {
                sh 'printenv | sort'
            }
        }
        stage('Hello') {
            steps {
                sh 'pwd'
                sh 'ls -l'
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
                sh 'Running Docker Test. Will save the log as artifact (docker_test.log).'
                sh 'make docker-test > jenkins_logs/docker_test.log 2>&1'
            }
        }
        stage('System Test') {
            steps {
                sh 'Running System Test. Will save the log as artifact (system_test.log).'
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
            archiveArtifacts artifacts: 'jenkins_logs/*', allowEmptyArchive: true
        }
    }
}
